package constants

const (
	MessageKindSize = 1
	// messageKindBufferSize = 1024
	ParamBufferSize = 1024
	HeaderSize      = 10
	HeaderParamSize = HeaderSize + ParamBufferSize
)

var HeaderPrefix = []byte("BEGINMESG")

type MessageKind byte

const (
	HelloMessage   MessageKind = 0x00
	HiMessage      MessageKind = 0x01
	PutFileMessge  MessageKind = 0x02
	GetFileMessage MessageKind = 0x03
)
