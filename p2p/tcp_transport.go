package p2p

import (
	"errors"
	"fmt"
	"net"
	"sync"
)

// remote node connected through tcp protocol
type TCPPeer struct {
	net.Conn

	// represents a client in 2 way communication
	// if it accepts the connection, it is server and outbound is false
	outbound bool
	Wg       *sync.WaitGroup
}

func NewTCPPeer(conn net.Conn, outbound bool) TCPPeer {
	return TCPPeer{
		Conn:     conn,
		outbound: outbound,
		Wg:       &sync.WaitGroup{},
	}
}

func (p TCPPeer) Send(b []byte) error {
	_, err := p.Conn.Write(b)
	return err
}

type TCPTransportOpts struct {
	Address    string
	ShakeHands HandshakeFunc
	Decoder    Decoder
	OnPeer     func(Peer) error
	Dispatcher func(Peer, bool)
}

type TCPTransport struct {
	TCPTransportOpts
	listener net.Listener
	rpcch    chan RPC
}

func NewTCPTransport(opts TCPTransportOpts) *TCPTransport {
	return &TCPTransport{
		TCPTransportOpts: opts,
		rpcch:            make(chan RPC),
	}
}

func (t *TCPTransport) Consume() <-chan RPC {
	return t.rpcch
}

func (t *TCPTransport) Close() error {
	return t.listener.Close()
}

func (t *TCPTransport) Dial(addr string) error {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return err
	}
	go t.handleConn(conn, true)
	return nil
}

func (t *TCPTransport) ListenAndAcceptSync(handler func(net.Conn)) {
	var err error
	fmt.Printf("Listening at %v\n", t.Address)
	t.listener, err = net.Listen("tcp", t.Address)
	if err != nil {
		fmt.Printf("Error on creating a connection %v\n", err)
	}
	for {
		conn, err := t.listener.Accept()
		if errors.Is(err, net.ErrClosed) {
			return
		}
		if err != nil {
			fmt.Printf("Error occurred while accepting connection %v", err)
		}

		go handler(conn)
	}
}

func (t *TCPTransport) ListenAndAccept() error {
	var err error
	fmt.Printf("Listening at %v\n", t.Address)
	t.listener, err = net.Listen("tcp", t.Address)
	if err != nil {
		return err
	}
	go t.acceptAndLoop()
	return nil
}

func (t *TCPTransport) acceptAndLoop() {
	for {
		conn, err := t.listener.Accept()
		if errors.Is(err, net.ErrClosed) {
			return
		}
		if err != nil {
			fmt.Printf("Error occurred while accepting connection %v", err)
		}

		go t.handleConn(conn, false)
	}
}

func (t *TCPTransport) handleConn(conn net.Conn, outbound bool) {
	if !outbound {
		fmt.Printf("Connection received from %v -> %s\n", conn.RemoteAddr().String(), conn.LocalAddr().String())
	}
	var err error
	defer func() {
		fmt.Printf("Dropping peer connection %v", err)
		conn.Close()
	}()
	peer := NewTCPPeer(conn, outbound)
	if err = t.ShakeHands(peer); err != nil {
		conn.Close()
		fmt.Printf("Error on shaking hands %v", err)
		return
	}
	if t.OnPeer != nil {
		if err = t.OnPeer(peer); err != nil {
			return
		}
	}
	// rpc := RPC{}
	t.Dispatcher(peer, outbound)
	//
	// for {
	// 	time.Sleep(time.Second * 2)
	// 	err = t.Decoder.Decode(conn, &rpc)
	// 	if errors.Is(err, io.EOF) {
	// 		return
	// 	}
	// 	if err != nil {
	// 		fmt.Printf("Error on decoding the data %v", err)
	// 		continue
	// 	}
	//
	// 	rpc.From = conn.RemoteAddr()
	// 	peer.Wg.Add(1)
	// 	t.rpcch <- rpc
	// 	peer.Wg.Wait()
	// 	fmt.Println("Waiting done!!")
	// }
}
