package chat

import (
	"fmt"

	rxi "github.com/bvisness/chat/serialization"
	"github.com/bvisness/chat/utils"
)

// ============================================================================
// Everything here is heavily based on aolo2's chat program:
// https://github.com/aolo2/chat/blob/master/src/websocket/websocket.h
// ============================================================================

/*
NOTE(ben): Events vs. Records

Events are the data actually sent as WebSocket messages. They are "ephemeral",
in the sense that they are understood to be unreliable against application-
level errors, such as the server crashing while handling them. Of course, since
they are sent over TCP, they are still reliably transmitted as long as the
connection remains alive, and will block other messages from being handled.

Records are data that make up the core, server-authoritative, super-reliable
stream of chat data. For each channel there is a reliable stream of records
that is synchronized to all clients. Clients can send new records to the
server; they will be added to the core record stream and retransmitted by the
server. (TODO: Correlate these messages using a hash or something.)

Both of these are dancing around the name "message", which we keep reserved for
chat messages to reduce cognitive burden.
*/

type EventType byte

const (
	ETRecord EventType = 0x01

	ETSYN EventType = 0x10
	ETACK EventType = 0x11

	ETTyping         EventType = 0x20
	ETPresenceUpdate EventType = 0x21

	// Client sending the server authentication info.
	ETAuth EventType = 0x90

	// Server -> client arbitrary error message.
	ETError    EventType = 0xFE
	ETReserved EventType = 0xFF
)

type Event struct {
	Type EventType

	// Type-specific payloads
	Record *Record
	SYNACK *EventSYNACK
	Error  *EventError
}

type EventSYNACK struct {
	SN int64
}

type EventError struct {
	Message string
}

func CreateSYNEvent(sn int64) Event {
	return Event{
		Type: ETSYN,
		SYNACK: &EventSYNACK{
			SN: sn,
		},
	}
}

func CreateErrorEvent(msg string, args ...any) Event {
	return Event{
		Type: ETError,
		Error: &EventError{
			Message: fmt.Sprintf(msg, args...),
		},
	}
}

func (e Event) Write(w *rxi.Writer) {
	w.Object()
	w.FieldByte("type", byte(e.Type))
	switch e.Type {
	case ETRecord:
		utils.Assert(e.Record, "missing payload for event of type", e.Type)
		w.FieldWritable("record", e.Record)
	case ETSYN, ETACK:
		utils.Assert(e.SYNACK, "missing payload for event of type", e.Type)
		w.FieldS64("sn", e.SYNACK.SN)
	case ETError:
		utils.Assert(e.Error, "missing payload for event of type", e.Type)
		w.FieldStr("message", e.Error.Message)
	default:
		panic(fmt.Errorf("cannot serialize event of type %v", e.Type))
	}
	w.End()
}

func ReadEvent(r *rxi.Reader) (Event, error) {
	var res Event
	evtObj, err := r.Object()
	if err != nil {
		return Event{}, err
	}

	// The "type" field must come first, always.
	rawType, err := r.FloatField(rxi.StrVal("type"))
	if err != nil {
		return Event{}, fmt.Errorf("type must be first field: %w", err)
	}
	res.Type = EventType(rawType)

	switch res.Type {
	case ETRecord:
		for key, recObj := range r.IterObject(evtObj) {
			var err error
			if rxi.FieldHasType(key, rxi.StrVal("record"), recObj, rxi.STObject, &err) {
				rec, err := IterRecord(r, recObj)
				if err != nil {
					return Event{}, err
				}
				res.Record = &rec
			} else if err != nil {
				return Event{}, err
			}
		}

	case ETSYN, ETACK:
		var payload EventSYNACK
		res.SYNACK = &payload
		for key, val := range r.IterObject(evtObj) {
			var err error
			if rxi.FieldHasType(key, rxi.StrVal("sn"), val, rxi.STFloat, &err) {
				payload.SN = int64(val.F)
				if payload.SN < 0 {
					return Event{}, fmt.Errorf("ACK SN cannot be negative: %d", payload.SN)
				}
			} else if err != nil {
				return Event{}, err
			}
		}
	}
	return res, nil
}

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

type Record struct {
	// A global sequence number identifying this record. For server -> client
	// communication, this SN is an authoritative ID for the record, shared by
	// all clients, and is always increasing, meaning that clients can always
	// order server records by SN. For client -> server communication, this SN is
	// unique to the session and is used to guarantee reliability against
	// application level failures in the server (e.g. the server crashing after
	// receiving a record but before writing it to disk).
	//
	// This implies that the SN of a record will be different when the client
	// receives it back from the server in its event stream.
	SN   int64
	Type RecordType

	// Message properties
	MessageText string
}

func (r Record) Write(w *rxi.Writer) {
	w.Object()
	if r.SN != 0 {
		w.FieldS64("sn", r.SN)
	}
	w.FieldByte("type", byte(r.Type))
	switch r.Type {
	case RTMessage:
		w.FieldStr("text", r.MessageText)
	default:
		panic(fmt.Errorf("in Record.Write: cannot write record type %v", r.Type))
	}
	w.End()
}

func ReadRecord(r *rxi.Reader) (Record, error) {
	obj, err := r.Object()
	if err != nil {
		return Record{}, err
	}
	rec, err := IterRecord(r, obj)
	if err != nil {
		return Record{}, err
	}
	return rec, nil
}

func IterRecord(r *rxi.Reader, obj rxi.Val) (Record, error) {
	var res Record

	// The "type" field must come first, always.
	rawType, err := r.FloatField(rxi.StrVal("type"))
	if err != nil {
		return Record{}, fmt.Errorf("type must be first field: %w", err)
	}
	res.Type = RecordType(rawType)

	switch res.Type {
	case RTMessage:
		for key, val := range r.IterObject(obj) {
			var err error
			if rxi.FieldHasType(key, rxi.StrVal("text"), val, rxi.STString, &err) {
				res.MessageText = val.S
			} else if err != nil {
				return Record{}, err
			}
		}
	default:
		return Record{}, fmt.Errorf("unknown record type %d", res.Type)
	}

	return res, nil
}
