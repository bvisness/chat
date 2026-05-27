package chat

import (
	"fmt"

	"github.com/bvisness/chat/utils"
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

var newRecordNotifications utils.Notifier

func (r Record) Write(w *Writer) {
	w.Object()
	if r.SN != 0 {
		w.FieldS64("sn", r.SN)
	}
	w.FieldByte("type", byte(r.Type))
	switch r.Type {
	case RTMessage:
		w.FieldStr("messageText", r.MessageText)
	default:
		panic(fmt.Errorf("in Record.Write: cannot write record type %v", r.Type))
	}
	w.End()
}
