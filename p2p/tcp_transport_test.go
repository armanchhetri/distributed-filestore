package p2p

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTCPTransport(t *testing.T) {
	trOpts := TCPTransportOpts{
		Address:    ":4000",
		Decoder:    DefaultDecoder{},
		ShakeHands: NopHandshakeFunc,
	}
	listenAddress := ":4000"
	tr := NewTCPTransport(trOpts)

	assert.Equal(t, tr.Address, listenAddress)
	assert.Nil(t, tr.ListenAndAccept())
}
