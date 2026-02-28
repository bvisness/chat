package utils

import (
	"slices"
	"sync"
)

type Waiter chan struct{}

func NewWaiter() Waiter {
	return Waiter(make(chan struct{}, 1))
}

func (w Waiter) Wake() {
	select {
	case w <- struct{}{}:
	default:
	}
}

type Notifier struct {
	lock sync.RWMutex
	subs []Waiter
}

func (n *Notifier) Subscribe(w Waiter) {
	n.lock.Lock()
	defer n.lock.Unlock()
	n.subs = append(n.subs, w)
}

func (n *Notifier) Unsubscribe(w Waiter) {
	n.lock.Lock()
	defer n.lock.Unlock()

	at := -1
	for i, sub := range n.subs {
		if sub == w {
			at = i
		}
	}
	if at == -1 {
		panic("the given waiter was not subscribed")
	}
	n.subs = slices.Delete(n.subs, at, at+1)
}

func (n *Notifier) Notify() {
	n.lock.RLock()
	defer n.lock.RUnlock()

	for _, c := range n.subs {
		c.Wake()
	}
}
