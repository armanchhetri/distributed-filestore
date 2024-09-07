package p2p

import "net"

// a remotely connected node
type Peer interface {
	net.Conn
	Send([]byte) error
}

// handles communication between nodes, a mode of communication
// can be TCP, UDP, websockets ..
type Transport interface {
	ListenAndAccept() error
	ListenAndAcceptSync(func(net.Conn))
	// Consume will return a readonly channel from where the caller can read the message from another peer.
	Consume() <-chan RPC
	Close() error
	Dial(string) error
}
