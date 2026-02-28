package chat

import (
	"slices"
	"sync"
)

type Event struct {
	// A global sequence number identifying this event in the server event stream. This is
	// universally recognized and shared across all clients and also serves as a unique ID for the
	// event.
	SN   int64
	Type EventType

	// Message properties
	MessageText string
}

var Events []Event
var newEvents = make(chan Event)
var eventNotifications Notifier

// NOTE(ben): We pretend to be a database by having a single goroutine that is responsible for
// modifying Events. This is sort of a sledgehammer approach to atomicity.
func pretendToBeADatabase() {
	for event := range newEvents {
		event.SN = int64(len(Events))
		Events = append(Events, event)
		eventNotifications.Notify()
	}
}

func init() {
	go pretendToBeADatabase()
}

type Notifier struct {
	lock sync.RWMutex
	subs []chan struct{}
}

func (n *Notifier) Subscribe() <-chan struct{} {
	n.lock.Lock()
	defer n.lock.Unlock()

	c := make(chan struct{}, 1)
	n.subs = append(n.subs, c)
	return c
}

func (n *Notifier) Unsubscribe(c <-chan struct{}) {
	n.lock.Lock()
	defer n.lock.Unlock()

	at := -1
	for i, sub := range n.subs {
		if sub == c {
			at = i
		}
	}
	if at == -1 {
		panic("the thing was not subscribed at all!!")
	}
	close(n.subs[at])
	n.subs = slices.Delete(n.subs, at, at+1)
}

func (n *Notifier) Notify() {
	n.lock.RLock()
	defer n.lock.RUnlock()

	for _, c := range n.subs {
		// NOTE(ben): Send on the channel, or don't if the channel is already full (indicating that the
		// subscriber hasn't picked up the prior notification yet).
		select {
		case c <- struct{}{}:
		default:
		}
	}
}
