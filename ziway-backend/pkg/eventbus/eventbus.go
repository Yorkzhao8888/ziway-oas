// Package eventbus provides a simple in-process event bus for P0.
// In P1, this will be replaced by Kafka without changing MBS/BOS code.
package eventbus

import (
	"sync"
)

// InProcessBus is a simple pub/sub for P0 monolith.
type InProcessBus struct {
	mu          sync.RWMutex
	subscribers map[string][]func(event []byte)
}

// New creates a new in-process event bus.
func New() *InProcessBus {
	return &InProcessBus{
		subscribers: make(map[string][]func(event []byte)),
	}
}

// Publish sends an event to all subscribers of the given topic.
func (b *InProcessBus) Publish(topic string, event []byte) error {
	b.mu.RLock()
	handlers := b.subscribers[topic]
	b.mu.RUnlock()
	for _, h := range handlers {
		go h(event) // async delivery
	}
	return nil
}

// Subscribe registers a handler for the given topic.
func (b *InProcessBus) Subscribe(topic string, handler func(event []byte)) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subscribers[topic] = append(b.subscribers[topic], handler)
	return nil
}
