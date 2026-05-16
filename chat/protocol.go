package chat

import (
	"fmt"

	"github.com/bvisness/chat/utils"
)

// ============================================================================
// Everything here is heavily based on aolo2's chat program:
// https://github.com/aolo2/chat/blob/master/src/websocket/websocket.h
// ============================================================================

/*
NOTE(ben): Events vs. Records

Events are the data actually sent as WebSocket messages. They are "ephemeral", in the sense that
they are understood to be unreliable against application-level errors, such as the server crashing
while handling them. Of course, since they are sent over TCP, they are still reliably transmitted
as long as the connection remains alive, and will block other messages from being handled.

Records are data that make up the core, server-authoritative, super-reliable stream of chat data.
For each channel there is a reliable stream of records that is synchronized to all clients. Clients
can send new records to the server; they will be added to the core record stream and retransmitted
by the server. (TODO: Correlate these messages using a hash or something.)

Both of these are dancing around the name "message", which we keep reserved for chat messages to
reduce cognitive burden.
*/

type EventType byte

const (
	ETRecord EventType = 0x01

	ETSYN  EventType = 0x10
	ETACKK EventType = 0x11

	ETTyping         EventType = 0x20
	ETPresenceUpdate EventType = 0x21

	// Client sending the server authentication info.
	ETAuth EventType = 0x90

	// Server -> client arbitrary error message.
	ETError    EventType = 0xFE
	ETReserved EventType = 0xFF
)

func CreateSYNEvent(buf []byte) []byte {
	w := messageWriter{buf: buf}
	utils.Must(w.WriteByte(byte(ETSYN), "message type"))
	return w.Bytes()
}

func CreateErrorEvent(buf []byte, msg string, args ...any) []byte {
	w := messageWriter{buf: buf}
	utils.Must(w.WriteByte(byte(ETError), "message type"))
	utils.Must(w.WriteString(fmt.Sprintf(msg, args...), "error message"))
	return w.Bytes()
}

// TODO(ben): Whatever the protocol ends up being, I do feel it would be wise to design a binary
// protocol that is at least a little bit key/value. It would be very useful to be able to create a
// generic message inspector that could render messages that are well-formed but nonetheless
// invalid.

type RecordType byte

const (
	RTMessage        RecordType = 0
	RTDeleteMessage  RecordType = 1
	RTEdit           RecordType = 2
	RTReply          RecordType = 3 // TODO(ben): Seems like this could be a parameter on the message, probably.
	RTReactionAdd    RecordType = 4
	RTReactionRemove RecordType = 5

	RTReserved RecordType = 0xFF
)
