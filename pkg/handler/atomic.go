package handler

import (
	"sync"
	"sync/atomic"
)

// notifiableAtomicBool is a variant of atomic.Bool.
// It can notify updates via channels.
type notifiableAtomicBool struct {
	atomic.Bool
	subscribers   []chan bool
	subscribersMu sync.RWMutex
}

func (x *notifiableAtomicBool) notify(val bool) {
	x.subscribersMu.RLock()
	for _, ch := range x.subscribers {
		ch <- val
	}
	x.subscribersMu.RUnlock()
}

func (x *notifiableAtomicBool) Store(val bool) {
	x.Bool.Store(val)
	x.notify(val)
}

func (x *notifiableAtomicBool) Swap(new bool) (old bool) {
	old = x.Bool.Swap(new)
	x.notify(new)
	return
}

func (x *notifiableAtomicBool) subscribe(ch chan bool) {
	x.subscribersMu.Lock()
	x.subscribers = append(x.subscribers, ch)
	x.subscribersMu.Unlock()
}

func (x *notifiableAtomicBool) Unsubscribe(ch chan bool) {
	x.subscribersMu.Lock()
	var new []chan bool
	for _, c := range x.subscribers {
		if c != ch {
			new = append(new, ch)
		}
	}
	x.subscribers = new
	x.subscribersMu.Unlock()
}
