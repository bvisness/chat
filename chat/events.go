package chat

import (
	"github.com/bvisness/chat/utils"
)

type Event struct {
	// A global sequence number identifying this event in the server event stream. This is
	// universally recognized and shared across all clients and also serves as a unique ID for the
	// event.
	SN   int64
	Type RecordType

	// Message properties
	MessageText string
}

func (e *Event) Serialize(w *eventWriter) error {
	if err := w.WriteS64(e.SN, "SN"); err != nil {
		return err
	}
	if err := w.WriteByte(byte(e.Type), "event type"); err != nil {
		return err
	}

	switch e.Type {
	case RTMessage:
		if err := w.WriteString(e.MessageText, "message text"); err != nil {
			return err
		}
	}

	return nil
}

var EventLog []Event
var newEvents = make(chan Event)
var newEventNotifications utils.Notifier

// NOTE(ben): We pretend to be a database by having a single goroutine that is responsible for
// modifying Events. This is sort of a sledgehammer approach to atomicity.
func pretendToBeADatabase() {
	for event := range newEvents {
		event.SN = int64(len(EventLog))
		EventLog = append(EventLog, event)
		newEventNotifications.Notify()
	}
}

func init() {
	go pretendToBeADatabase()
}
