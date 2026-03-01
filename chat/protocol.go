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
NOTE(ben): Messages vs. Events

Messages are the data actually sent as WebSocket messages. They are "ephemeral", in the sense that
they are understood to be unreliable against application-level errors, such as the server crashing
while handling them. Of course, since they are sent over TCP, they are still reliably transmitted
as long as the connection remains alive, and will block other messages from being handled.

Events are data that make up the core, super-reliable event stream between the client and server.
The server has _the_ authoritative event stream for each channel, and is primarily responsible for
delivering it reliably to all clients. Client to server communication also uses a reliable event
stream, but client events are a different stream from server events; the server will take the
client events, handle them, and then dispatch its own authoritative events as part of its normal
operation.

TODO(ben): Maybe in the future the server could explicitly inform the client of a mapping between
client events and server events. That would allow clients to optimistically render the result of
an action, and then roll back if the action was rejected. But this seems like a lot of complexity
that can be added at a later date, and it would be nice to design the protocol so that clients can
do the easy thing and fully rely on server-authoritative events for what gets rendered.
*/

type MessageType byte

const (
	// Persistent events belong to a specific event stream and will be made reliable at the
	// application level. For example, the server will only ACK a new-message event when the message
	// has been persisted to the database.
	MTPersistentEvent MessageType = 0x01

	MTTyping         MessageType = 0x20
	MTPresenceUpdate MessageType = 0x21

	// Acknowledging receipt of all events up to a particular SN.
	MTACK MessageType = 0x80
	// Explicitly requesting an ACK from the other side.
	MTACKPLZ MessageType = 0x81

	// Client sending the server authentication info.
	MTAuth MessageType = 0x90

	// Server -> client arbitrary error message.
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
