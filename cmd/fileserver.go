package main

import (
	"bytes"
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"sync"

	"github.com/armanchhetri/datastore/cmd/constants"
	"github.com/armanchhetri/datastore/p2p"
	"github.com/armanchhetri/datastore/packages/helpers"
)

type PutFileParam struct {
	Filename string
	Size     int64
	IsReply  bool
	ReplyID  string
	HasFile  bool
}

type GetFileParam struct {
	Filename string
	Offset   int64
}

type FileServerOpts struct {
	RootStoragePath       string
	PathTransformFunction PathTransformFunction
	Transport             p2p.Transport
	Administration        p2p.Transport
	BootstrapNodes        []string
}

type FileServer struct {
	FileServerOpts
	Store            *Store
	quitch           chan struct{}
	peerLock         sync.Mutex
	peers            map[string]p2p.Peer
	pendingToReceive map[string]struct{} // to track what files current server has asked for
	mu               sync.Mutex
}

func NewFileServer(opts FileServerOpts) *FileServer {
	storeOpts := StoreOpts{
		PathTransformFunction: opts.PathTransformFunction,
		Root:                  opts.RootStoragePath,
	}
	store := NewStore(storeOpts)
	return &FileServer{
		FileServerOpts:   opts,
		Store:            store,
		quitch:           make(chan struct{}),
		pendingToReceive: make(map[string]struct{}),
		peers:            make(map[string]p2p.Peer),
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

func (fs *FileServer) ConnectionDispatcher(peer p2p.Peer, outbound bool) {
	if outbound {
		fs.sayHello(peer, outbound)
	}
	for {
		messageKind, err := helpers.ReadUntilGetAfter(peer, constants.HeaderPrefix)
		if errors.Is(err, io.EOF) {
			return
		}
		if err != nil {
			fmt.Printf("Error while reading header %v\n", err)
			continue
		}

		switch constants.MessageKind(messageKind) {
		case constants.HelloMessage:
			fs.sayHello(peer, false)
		case constants.HiMessage:
			fmt.Printf("Handshake complete: hi received from %s \n", peer.RemoteAddr().String())
		case constants.PutFileMessge:
			fmt.Printf("Remote %s wants to put file to local %s\n", peer.RemoteAddr().String(), peer.LocalAddr().String())
			paramBuffer := make([]byte, constants.ParamBufferSize)

			_, err := io.ReadAtLeast(peer, paramBuffer, constants.ParamBufferSize)
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

		case constants.GetFileMessage:
			fmt.Printf("%s wants file from %s", peer.RemoteAddr().String(), peer.LocalAddr().String())
			paramBuffer := make([]byte, constants.ParamBufferSize)

			_, err := io.ReadAtLeast(peer, paramBuffer, constants.ParamBufferSize)
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

	// }
}

func (fs *FileServer) sayHello(peer p2p.Peer, outbound bool) error {
	message := constants.HelloMessage
	if !outbound {
		message = constants.HiMessage
	}
	header := fs.buildHeader(message)

	_, err := peer.Write(header[:])
	if err != nil {
		return fmt.Errorf("could not say hello %v", err)
	}
	return nil
}

func (fs *FileServer) handlePutfile(fileParam PutFileParam, peer p2p.Peer) error {
	// TODO: handle reply file
	if fileParam.IsReply {
		if !fileParam.HasFile {
			fmt.Printf("%s does not have file %s\n", peer.RemoteAddr().String(), fileParam.Filename)
			return nil
		}
		fs.mu.Lock()
		if _, ok := fs.pendingToReceive[fileParam.Filename]; !ok {
			fmt.Printf("File (%s) already received \n", fileParam.Filename)
			return nil
		}
		delete(fs.pendingToReceive, fileParam.Filename)

		fs.mu.Unlock()

	}
	limitFileStream := io.LimitReader(peer, fileParam.Size)
	return fs.Store.WriteStream(FileKey(fileParam.Filename), limitFileStream)
}

func (fs *FileServer) buildHeader(messageKind constants.MessageKind) [10]byte {
	var header [10]byte
	copy(header[:], constants.HeaderPrefix)
	header[constants.HeaderSize-1] = byte(messageKind)

	return header
}

func (fs *FileServer) buildMessage(header [constants.HeaderSize]byte, param [constants.ParamBufferSize]byte) [constants.HeaderParamSize]byte {
	msg := append(header[:], param[:]...)

	var message [constants.HeaderParamSize]byte
	copy(message[:], msg)
	return message
}

func (fs *FileServer) BroadCastFile(fileParam PutFileParam, fileStream io.Reader) error {
	peers := []io.Writer{}
	for _, peer := range fs.peers {
		peers = append(peers, peer)
	}
	multiWriter := io.MultiWriter(peers...)
	// TODO: send file in parallel

	header := fs.buildHeader(constants.PutFileMessge)
	params, err := fs.encodeParams(fileParam)
	if err != nil {
		return err
	}
	msg := fs.buildMessage(header, params)

	_, err = multiWriter.Write(msg[:])
	if err != nil {
		return err
	}

	_, err = io.Copy(multiWriter, fileStream)

	return err
}

func (fs *FileServer) handleGetFile(peer p2p.Peer, param GetFileParam) error {
	putFileParam := PutFileParam{IsReply: true, Filename: param.Filename}
	fileExists := fs.Store.Has(FileKey(param.Filename))
	if fileExists {
		putFileParam.HasFile = true
		fileInfo, err := fs.Store.Stat(FileKey(param.Filename))
		if err != nil {
			return err
		}
		putFileParam.Size = fileInfo.Size()
	}

	params, err := fs.encodeParams(putFileParam)
	if err != nil {
		return err
	}

	header := fs.buildHeader(constants.PutFileMessge)
	msg := fs.buildMessage(header, params)

	_, err = peer.Write(msg[:])
	if err != nil {
		return err
	}

	if fileExists {
		return fs.Store.StreamFile(FileKey(param.Filename), peer)
	}
	return nil
}

func (fs *FileServer) GetFileRemote(key string) error {
	getFileParam := GetFileParam{
		Filename: key,
	}
	params, err := fs.encodeParams(getFileParam)
	if err != nil {
		return nil
	}

	header := fs.buildHeader(constants.GetFileMessage)
	msg := fs.buildMessage(header, params)

	fs.mu.Lock()
	if _, ok := fs.pendingToReceive[key]; !ok {
		fs.pendingToReceive[key] = struct{}{}
	}
	fs.mu.Unlock()

	for _, peer := range fs.peers {
		_, err = peer.Write(msg[:])
		if err != nil {
			fmt.Printf("Could not contact peer %s for a file\n", peer.RemoteAddr())
			continue
		}
	}
	return nil
}

func (fs *FileServer) loop() {
	defer func() {
		fmt.Println("Stopping the file server")
		err := fs.Transport.Close()
		if err != nil {
			log.Fatalf("Error while closing the transport (%s)", err.Error())
		}
	}()
	fs.Administration.ListenAndAcceptSync(fs.AdminHandler)
}

func (fs *FileServer) SaveFile(filename string, r io.Reader) error {
	// save locally and broadcast
	fileKey := FileKey(filename)
	err := fs.Store.WriteStream(fileKey, r)
	if err != nil {
		return err
	}
	stat, err := fs.Store.Stat(fileKey)
	if err != nil {
		return err
	}

	f, err := fs.Store.GetFile(FileKey(filename))
	if err != nil {
		fmt.Printf("Error on reading file for broadcasting, %v\n", err)
		return err
	}
	putFileParams := PutFileParam{
		Filename: stat.Name(),
		Size:     stat.Size(),
		IsReply:  false,
	}

	return fs.BroadCastFile(putFileParams, f)
}

func (fs *FileServer) AdminHandler(conn net.Conn) {
	fmt.Printf("Admin connection from %s \n", conn.RemoteAddr().String())
	for {
		commBuf, err := helpers.ReadUntilAndGet(conn, []byte{0x00})
		// _, err := io.ReadAtLeast(conn, commBuf, commBufSize)
		if errors.Is(err, io.EOF) {
			return
		}
		if err != nil {
			fmt.Printf("Error while reading command %v\n", err)
			return
		}
		commParam := strings.Split(string(commBuf[:len(commBuf)-1]), " ")

		command := commParam[0]

		switch command {
		case "ls":
			var files []string
			for key := range fs.Store.Files {
				files = append(files, string(key))
			}
			output := strings.Join(files, "\n")
			_, err := conn.Write([]byte(output))
			if err != nil {
				fmt.Printf("Error on ls %v\n", err)
				return
			}

		case "write":
			if len(commParam) < 2 {
				_, err := conn.Write([]byte("Provide filename to write, eg: write filename"))
				if err != nil {
					fmt.Printf("Error on writing the connection %v\n", err)
				}
				return
			}
			filename := commParam[1]
			err := fs.SaveFile(filename, conn)
			if err != nil {
				fmt.Printf("Error on writing the file %s \n", filename)
				_, err = conn.Write([]byte("Could not save the give file"))
				if err != nil {
					fmt.Printf("Error on writing the connection %v\n", err)
				}
				return
			}
			_, err = fmt.Fprintf(conn, "successfully stored %s", filename)
			if err != nil {
				fmt.Printf("error on writing to the connection %v\n", err)
			}

		case "cat":
			if len(commParam) < 2 {
				_, err := conn.Write([]byte("Need to provide filename for to cat, eg: cat abc.txt"))
				if err != nil {
					fmt.Printf("Error on writing to the connection, %v\n", err)
					return
				}
			}
			filename := commParam[1]
			if !fs.Store.Has(FileKey(filename)) {
				// _, err := conn.Write([]byte(fmt.Sprintf("No such file %s", filename)))

				_, err := fmt.Fprintf(conn, "No such file %s", filename)
				if err != nil {
					fmt.Printf("Error on writing to the connection %v\n", err)
					return
				}
			}
			err := fs.Store.StreamFile(FileKey(filename), conn)
			if err != nil {
				fmt.Printf("Error on writing file to the connection %v", err)
			}

		default:
			fmt.Fprintf(conn, "Invalid command %s", command)
			return
		}
	}
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

func (fs *FileServer) decodeParams(b []byte, param any) error {
	paramReader := bytes.NewReader(b)
	dec := gob.NewDecoder(paramReader)
	return dec.Decode(param)
}

func (fs *FileServer) encodeParams(param any) ([constants.ParamBufferSize]byte, error) {
	var params [constants.ParamBufferSize]byte
	var buffer bytes.Buffer
	enc := gob.NewEncoder(&buffer)
	err := enc.Encode(param)
	if err != nil {
		return params, err
	}

	paramsBuffer := helpers.PadBytes(buffer.Bytes(), constants.ParamBufferSize)
	copy(params[:], paramsBuffer)
	return params, nil
}

func init() {
	gob.Register(GetFileParam{})
}
