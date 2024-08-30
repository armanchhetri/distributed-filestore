package main

import (
	"bytes"
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"log"
	"sync"
	"time"

	"github.com/armanchhetri/datastore/p2p"
	"github.com/armanchhetri/datastore/packages/helpers"
)

type PutFileParam struct {
	Filename string
	Size     int64
}

type GetFileParam struct {
	Filename string
	Offset   int64
}

type GetFileReply struct {
	GetFileParam
	Size    int64
	HasFile bool
}

type FileServerOpts struct {
	RootStoragePath       string
	PathTransformFunction PathTransformFunction
	Transport             p2p.Transport
	BootstrapNodes        []string
}

type FileServer struct {
	FileServerOpts
	Store      *Store
	quitch     chan struct{}
	peerLock   sync.Mutex
	peers      map[string]p2p.Peer
	readToggle chan struct{}
}

func NewFileServer(opts FileServerOpts) *FileServer {
	storeOpts := StoreOpts{
		PathTransformFunction: opts.PathTransformFunction,
		Root:                  opts.RootStoragePath,
	}
	store := NewStore(storeOpts)
	return &FileServer{
		FileServerOpts: opts,
		Store:          store,
		quitch:         make(chan struct{}),
		readToggle:     make(chan struct{}),
		peers:          make(map[string]p2p.Peer),
	}
}

func (fs *FileServer) bootstrapNetwork() error {
	for _, addr := range fs.BootstrapNodes {
		if len(addr) == 0 {
			continue
		}
		go func(addr string) {
			fmt.Printf("Connecting to remote %s\n", addr)
			err := fs.Transport.Dial(addr)
			if err != nil {
				fmt.Println(err)
			}
		}(addr)
	}
	return nil
}

func (fs *FileServer) OnPeer(p p2p.Peer) error {
	fs.peerLock.Lock()
	defer fs.peerLock.Unlock()
	fs.peers[p.RemoteAddr().String()] = p
	log.Printf("Peer connected with remote addr: %v", p.RemoteAddr())
	return nil
}

type (
	MessageStoreKey string
)

type Message struct {
	Payload any
}

type MessageStoreFile struct {
	Key  string
	Size int64
}

// type MessageStoreKey struct {
// 	Key  string
// }

func (fs *FileServer) broadcast(m Message) error {
	peers := []io.Writer{}
	for _, peer := range fs.peers {
		peers = append(peers, peer)
	}
	mw := io.MultiWriter(peers...)
	// gob.Register(m)
	return gob.NewEncoder(mw).Encode(m)
}

func (fs *FileServer) broadcastFile(r io.Reader) error {
	// fmt.Println("sending: ", f)
	for _, peer := range fs.peers {
		// err := peer.Send(f)
		_, err := io.Copy(peer, r)
		if err != nil {
			return err
		}
	}
	return nil
}

func (fs *FileServer) StoreData(key string, r io.Reader) error {
	// buf := new(bytes.Buffer)
	// io.TeeReader(r, buf)

	// if err := fs.store.WriteStream(FileKey(key), teeR); err != nil {
	// 	return err
	// }

	msg := Message{
		MessageStoreFile{
			Key:  key,
			Size: 40,
		},
	}
	// keyMsg := Message(MessageStoreKey(key))

	fs.broadcast(msg)
	time.Sleep((time.Second * 5))

	// dataMsg := Message{buf.Bytes()}

	err := fs.broadcastFile(r)
	fmt.Println("Broadcasted", err)
	if err != nil {
		log.Fatal(err)
	}
	return nil
}

const (
	messageKindSize       = 1
	messageKindBufferSize = 1024
	paramBufferSize       = 1024
	headerSize            = 10
)

var headerPrefix = []byte("BEGINMESG")

type MessageKind byte

const (
	helloMessage   MessageKind = 0x00
	hiMessage      MessageKind = 0x01
	putFileMessge  MessageKind = 0x02
	getFileMessage MessageKind = 0x03
)

func (fs *FileServer) readUntilHeader(r io.Reader) (MessageKind, error) {
	buf := make([]byte, 1)
	subKey := 0

	for {
		fmt.Println("reading until header", string(buf))
		_, err := r.Read(buf)
		if err != nil {
			return 0, err
		}
		ch2 := buf[0]
		if headerPrefix[subKey] == ch2 {
			subKey++
			if subKey == len(headerPrefix) {
				break
			}
		} else if subKey != 0 {
			subKey = 0
		}
	}
	_, err := r.Read(buf)

	return MessageKind(buf[0]), err
}

func (fs *FileServer) ConnectionDispatcher(peer p2p.Peer, outbound bool) {
	if outbound {
		fs.sayHello(peer, outbound)
	}
	for {
		// buf := make([]byte, 10)
		// peer.Read(buf)
		// fmt.Printf("%v", buf)
		// panic("hello")
		byteReader := helpers.NewByteReader(peer)
		hasData := make(chan struct{})
		hasError := make(chan error)
		go func(hasData chan struct{}, hasError chan error) {
			_, err := byteReader.ReadByte()
			if err != nil {
				hasError <- err
			}
			byteReader.UnreadByte()
			hasData <- struct{}{}
		}(hasData, hasError)

		select {
		case <-fs.readToggle:
			fmt.Printf("Stopping reading in %s\n", peer.LocalAddr().String())
			<-fs.readToggle
			fmt.Println("resuming reading")
		case err := <-hasError:
			if errors.Is(err, io.EOF) {
				return
			}
		case <-hasData:
			messageKind, err := fs.readUntilHeader(&byteReader)
			if errors.Is(err, io.EOF) {
				return
			}
			if err != nil {
				fmt.Printf("Error while reading header %v\n", err)
				continue
			}

			switch messageKind {
			case helloMessage:
				fs.sayHello(peer, false)
			case hiMessage:
				fmt.Printf("Handshake complete: hi received from %s \n", peer.RemoteAddr().String())
			case putFileMessge:
				fmt.Printf("Remote %s wants to put file to local %s\n", peer.RemoteAddr().String(), peer.LocalAddr().String())
				paramBuffer := make([]byte, paramBufferSize)

				_, err := io.ReadAtLeast(peer, paramBuffer, paramBufferSize)
				if err != nil {
					fmt.Printf("Invalid putfile message got (%v)\n", err)
				}
				var param PutFileParam
				err = fs.decodeParams(paramBuffer, &param)
				if err != nil {
					fmt.Printf("error while decoding putFileParam %v\n", err)
					continue
				}

				fs.handlePutfile(param, peer)

			case getFileMessage:
				fmt.Printf("%s wants file from %s", peer.RemoteAddr().String(), peer.LocalAddr().String())
				paramBuffer := make([]byte, paramBufferSize)
				// buf := make([]byte, 1)
				// peer.Read(buf)
				// fmt.Println(buf)

				_, err := io.ReadAtLeast(peer, paramBuffer, paramBufferSize)
				if err != nil {
					fmt.Printf("could not read param buffer from connection %v\n", err)
				}
				var param GetFileParam
				err = fs.decodeParams(paramBuffer, &param)
				if err != nil {
					fmt.Printf("Could not decode parameters from bytes %v\n", err)
				}

				fs.handleGetFile(peer, param)

				fmt.Println("\nfile sent")

			default:
				fmt.Printf("Invalid communication message %v", messageKind)
			}
		}

	}
}

func (fs *FileServer) decodeParams(b []byte, param any) error {
	paramReader := bytes.NewReader(b)
	dec := gob.NewDecoder(paramReader)
	return dec.Decode(param)
}

func pad(b []byte, size int) []byte {
	p := make([]byte, size)
	copy(p, b)
	return p
}

func (fs *FileServer) encodeParam(param any) ([]byte, error) {
	var buffer bytes.Buffer
	enc := gob.NewEncoder(&buffer)
	err := enc.Encode(param)
	if err != nil {
		return nil, err
	}
	return pad(buffer.Bytes(), paramBufferSize), nil
}

func (fs *FileServer) handleGetFile(peer p2p.Peer, param GetFileParam) error {
	var getFileReply GetFileReply
	fileExists := fs.Store.Has(FileKey(param.Filename))
	if fileExists {
		getFileReply.HasFile = true
		getFileReply.GetFileParam = param
		fileInfo, err := fs.Store.Stat(FileKey(param.Filename))
		if err != nil {
			return err
		}
		getFileReply.Size = fileInfo.Size()
	}
	paramBuffer, err := fs.encodeParam(getFileReply)
	if err != nil {
		return err
	}

	fmt.Println("streaming file")
	_, err = peer.Write(paramBuffer)
	if err != nil {
		return err
	}

	if fileExists {
		return fs.Store.StreamFile(FileKey(param.Filename), peer)
	}
	return nil
}

func (fs *FileServer) handlePutfile(fileParam PutFileParam, fileStream io.Reader) error {
	limitFileStream := io.LimitReader(fileStream, fileParam.Size)
	return fs.Store.WriteStream(FileKey(fileParam.Filename), limitFileStream)
}

func (fs *FileServer) sayHello(peer p2p.Peer, outbound bool) error {
	var message MessageKind = helloMessage
	if !outbound {
		message = hiMessage
	}
	command := append(headerPrefix, byte(message))
	_, err := peer.Write(command)
	if err != nil {
		return fmt.Errorf("could not say hello %v", err)
	}
	return nil
}

func (fs *FileServer) BroadCastFile(fileParam PutFileParam, fileStream io.Reader) error {
	peers := []io.Writer{}
	for _, peer := range fs.peers {
		peers = append(peers, peer)
	}
	multiWriter := io.MultiWriter(peers...)
	// TODO: send file in parallel
	var buffer bytes.Buffer
	enc := gob.NewEncoder(&buffer)
	err := enc.Encode(fileParam)
	if err != nil {
		return err
	}
	// pad upto messgaekindbuffer size
	fullData := make([]byte, messageKindBufferSize+headerSize)

	msg := append(headerPrefix, byte(putFileMessge))
	command := append(msg, buffer.Bytes()...)

	copy(fullData, command)
	_, err = multiWriter.Write(fullData)
	if err != nil {
		return err
	}

	_, err = io.Copy(multiWriter, fileStream)

	return err
}

func (fs *FileServer) ReadInFromRemote(key string, w io.Writer) error {
	fs.readToggle <- struct{}{}
	r, err := fs.GetFileRemote(key)
	if err != nil {
		fs.readToggle <- struct{}{}
		return err
	}
	_, err = io.Copy(w, r)

	fs.readToggle <- struct{}{}
	return err
}

func (fs *FileServer) GetFileRemote(key string) (io.Reader, error) {
	getFileParam := GetFileParam{
		Filename: key,
	}
	getParamBytes, err := fs.encodeParam(getFileParam)
	if err != nil {
		return nil, err
	}
	header := append(headerPrefix, byte(getFileMessage))
	fullMessage := append(header, getParamBytes...)

	responseParamBuffer := make([]byte, paramBufferSize)

	for _, peer := range fs.peers {
		_, err = peer.Write(fullMessage)
		if err != nil {
			fmt.Printf("Could not contact peer %s for a file\n", peer.RemoteAddr())
			continue
		}
		// err = peer.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
		// if err != nil {
		// 	fmt.Printf("Error on setting connection deadline %v", err)
		// 	continue
		// }
		time.Sleep(time.Second * 2)
		fmt.Println("\nread file param")
		buf := make([]byte, 20)
		peer.Read(buf)
		fmt.Println(string(buf))

		_, err = io.ReadAtLeast(peer, responseParamBuffer, paramBufferSize)
		if err != nil {
			fmt.Printf("error while reading parameters %v\n", err)

			continue
		}
		var getFileReply GetFileReply
		fmt.Println(string(responseParamBuffer))
		err = fs.decodeParams(responseParamBuffer, &getFileReply)
		if err != nil {
			fmt.Printf("Couldn not decode reply param %v\n", err)
		}

		if getFileReply.HasFile {
			return io.LimitReader(peer, getFileReply.Size), nil
		}
	}
	fmt.Println("Could not find file in any registered peers")
	return nil, fmt.Errorf("could not get file from any peers")
}

func (fs *FileServer) SendFile(fileStream io.Reader, destination io.Writer) error {
	_, err := io.Copy(destination, fileStream)
	return err
}

func (fs *FileServer) loop() {
	defer func() {
		fmt.Println("Stopping the file server")
		err := fs.Transport.Close()
		if err != nil {
			log.Fatalf("Error while closing the transport (%s)", err.Error())
		}
	}()

	for {
		select {
		case rpc := <-fs.Transport.Consume():
			fmt.Println(rpc.Payload)
			// fmt.Println("Here")
			var m Message
			if err := gob.NewDecoder(bytes.NewReader(rpc.Payload)).Decode(&m); err != nil {
				log.Fatal(err)
			}
			fs.handleMessage(rpc.From.String(), &m)

		case <-fs.quitch:

			fmt.Println("Quiting")
			return
		}
	}
}

func (fs *FileServer) handleMessage(from string, msg *Message) error {
	fmt.Printf("Message== %+v\n", msg)

	switch msg.Payload.(type) {
	case MessageStoreFile:
		fs.handleMessageStoreFile(from, msg.Payload.(MessageStoreFile))
	}
	// if dmsg, ok := msg.Payload.(*DataMessage); ok {
	// 	fmt.Printf("%+v\n", dmsg)
	// }
	return nil
}

func (fs *FileServer) handleMessageStoreFile(from string, msg MessageStoreFile) error {
	fmt.Println("Here...")
	peer, ok := fs.peers[from]
	if !ok {
		return errors.New("peer not found")
	}
	// buf := make([]byte, 1024)
	// _, err := peer.Read(buf)
	// if err != nil {
	// 	fmt.Println(err)
	// }
	// fmt.Println(string(buf))
	if err := fs.Store.WriteStream(FileKey(msg.Key), io.LimitReader(peer, msg.Size)); err != nil {
		fmt.Println("Error here: ", err)
		return err
	}
	peer.(p2p.TCPPeer).Wg.Done()
	return nil
}

func (fs *FileServer) Stop() {
	close(fs.quitch)
}

func (fs *FileServer) Start() error {
	err := fs.Transport.ListenAndAccept()
	fs.bootstrapNetwork()
	fs.loop()
	return err
}

func init() {
	gob.Register(MessageStoreFile{})
}
