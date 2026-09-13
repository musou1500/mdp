package server

import (
	"sync"
)

// hub broadcasts change signals to the connected event streams. Closing a
// channel is the only broadcast Go has, and it fires once, so each broadcast
// closes the current channel and installs a fresh one for the next wait. No
// subscriber registry is needed: the signal carries nothing, so every waiter
// wants the same thing.
type hub struct {
	mu      sync.Mutex
	changed chan struct{}
}

func newHub(changes <-chan struct{}) *hub {
	h := &hub{changed: make(chan struct{})}
	go func() {
		for range changes {
			h.broadcast()
		}
	}()
	return h
}

// wait returns a channel that the next change closes. Callers reread it after
// every change: the one they were holding is spent.
func (h *hub) wait() <-chan struct{} {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.changed
}

func (h *hub) broadcast() {
	h.mu.Lock()
	defer h.mu.Unlock()
	close(h.changed)
	h.changed = make(chan struct{})
}
