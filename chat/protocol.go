package chat

import "fmt"

// ============================================================================
// Everything here is heavily based on aolo2's chat program:
// https://github.com/aolo2/chat/blob/master/src/websocket/websocket.h
// ============================================================================

type EventType byte

const (
	ETMessage        EventType = 0
	ETDeleteMessage  EventType = 1
	ETEdit           EventType = 2
	ETReply          EventType = 3 // TODO(ben): Seems like this could be a parameter on the message, probably.
	ETReactionAdd    EventType = 4
	ETReactionRemove EventType = 5
)

type MessageType byte

const (
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
