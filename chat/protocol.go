package chat

import "fmt"

// ============================================================================
// Everything here is heavily based on aolo2's chat program:
// https://github.com/aolo2/chat/blob/master/src/websocket/websocket.h
// ============================================================================

type MessageType byte

const (
	MTMessage        MessageType = 0
	MTDeleteMessage  MessageType = 1
	MTEdit           MessageType = 2
	MTReply          MessageType = 3 // TODO(ben): Seems like this could be a parameter on the message, probably.
	MTReactionAdd    MessageType = 4
	MTReactionRemove MessageType = 5

	// TODO(ben): Need to make a distinction between WebSocket protocol messages and application
	// events to be synced!
	MTError    MessageType = 0xFE
	MTReserved MessageType = 0xFF
)

func ErrorMessage(msg string, args ...any) []byte {
	formatted := fmt.Sprintf(msg, args...)
	res := make([]byte, 1+len(formatted))
	res[0] = byte(MTError)
	copy(res[1:], formatted)
	return res
}
