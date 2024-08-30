package main

import (
	"bytes"
	"fmt"
	"log"
	"time"

	"github.com/armanchhetri/datastore/p2p"
)

func makeServer(listenAddr string, nodes ...string) *FileServer {
	tcpTransportOpts := p2p.TCPTransportOpts{
		Address:    listenAddr,
		ShakeHands: p2p.NopHandshakeFunc,
		Decoder:    p2p.DefaultDecoder{},
	}
	tcpTransport := p2p.NewTCPTransport(tcpTransportOpts)

	fileServerOpts := FileServerOpts{
		RootStoragePath:       DEFAULT_STORAGE_PATH + "/" + listenAddr,
		PathTransformFunction: CASPathTransformFunc,
		Transport:             tcpTransport,
		BootstrapNodes:        nodes,
	}
	fileServer := NewFileServer(fileServerOpts)

	tcpTransport.OnPeer = fileServer.OnPeer
	tcpTransport.Dispatcher = fileServer.ConnectionDispatcher

	return fileServer
}

func main() {
	// s1 := makeServer("127.0.0.1:3000", "127.0.0.1:4000", "127.0.0.1:6000")
	s1 := makeServer("127.0.0.1:3000", "127.0.0.1:4000")

	s2 := makeServer("127.0.0.1:4000", "")

	// s3 := makeServer("127.0.0.1:6000", "")

	go func() {
		log.Fatal(s2.Start())
	}()

	// go func() {
	// 	log.Fatal(s3.Start())
	// }()
	//
	time.Sleep(2 * time.Second)
	go s1.Start()
	time.Sleep(1 * time.Second)
	// data := []byte("this is a big data file to be written")
	// dataReader := bytes.NewReader(data)
	// size := len(data)
	//
	// param := PutFileParam{
	// 	Filename: "testfilename",
	// 	Size:     int64(size),
	// }
	// s1.BroadCastFile(param, dataReader)

	var buffer bytes.Buffer
	filename := "testfilename"

	err := s1.ReadInFromRemote(filename, &buffer)
	if err != nil {
		log.Fatal(err)
	}
	// io.Copy(io.Writer(&buffer), r)
	fmt.Println(buffer.String())

	// s2.StoreData("mytestdatakey", file)
	select {}
}
