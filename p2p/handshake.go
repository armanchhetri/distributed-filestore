package p2p

type HandshakeFunc func(any) error

func NopHandshakeFunc(any) error {
	return nil
}
