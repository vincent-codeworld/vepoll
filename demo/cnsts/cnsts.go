package cnsts

type MessageType uint8

const (
	MessageTypeUnknown MessageType = iota
	MessageTypeText
	MessageTypePic
	MessageTypeVideo
	MessageTypeAck
)
