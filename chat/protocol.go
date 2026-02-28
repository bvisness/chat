package chat

import (
	"fmt"

	"github.com/bvisness/chat/utils"
)

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

	ETReserved EventType = 0xFF
)

type MessageType byte

const (
	// Initial transmission or retransmission of a persistent event.
	MTEvent MessageType = 0x01

	// Acknowledging receipt of all events up to a particular SN.
	MTACK MessageType = 0x80
	// Explicitly requesting an ACK from the other side.
	MTACKPLZ MessageType = 0x81

	MTError    MessageType = 0xFE
	MTReserved MessageType = 0xFF
)

func MessageACKPLZ(buf []byte) []byte {
	w := messageWriter{buf: buf}
	utils.Must(w.WriteByte(byte(MTACKPLZ), "message type"))
	return w.Bytes()
}

func MessageError(buf []byte, msg string, args ...any) []byte {
	w := messageWriter{buf: buf}
	utils.Must(w.WriteByte(byte(MTError), "message type"))
	utils.Must(w.WriteString(fmt.Sprintf(msg, args...), "error message"))
	return w.Bytes()
}

// TODO(ben): Whatever the protocol ends up being, I do feel it would be wise to design a binary
// protocol that is at least a little bit key/value. It would be very useful to be able to create a
// generic message inspector that could render messages that are well-formed but nonetheless
// invalid.
