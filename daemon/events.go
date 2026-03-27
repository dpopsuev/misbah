package daemon

import (
	"sync"
	"time"
)

// ContainerEventType identifies the kind of lifecycle event.
type ContainerEventType string

const (
	EventStarted ContainerEventType = "started"
	EventStopped ContainerEventType = "stopped"
	EventExited  ContainerEventType = "exited"
	EventError   ContainerEventType = "error"
)

// ContainerEvent represents a container lifecycle event.
type ContainerEvent struct {
	Type      ContainerEventType `json:"type"`
	Name      string             `json:"name"`
	ExitCode  int32              `json:"exit_code,omitempty"`
	Error     string             `json:"error,omitempty"`
	Timestamp int64              `json:"timestamp"`
}

// EventBus is a simple pub-sub for container lifecycle events.
// Subscribers receive events on channels. Thread-safe.
type EventBus struct {
	mu          sync.RWMutex
	subscribers map[string][]chan ContainerEvent // keyed by container name, "" = all
}

// NewEventBus creates an EventBus.
func NewEventBus() *EventBus {
	return &EventBus{
		subscribers: make(map[string][]chan ContainerEvent),
	}
}

// Subscribe returns a channel that receives events for the given container.
// Pass empty name to receive all events. Caller must call Unsubscribe when done.
func (b *EventBus) Subscribe(name string) chan ContainerEvent {
	ch := make(chan ContainerEvent, 16)
	b.mu.Lock()
	b.subscribers[name] = append(b.subscribers[name], ch)
	b.mu.Unlock()
	return ch
}

// Unsubscribe removes a channel from the bus and closes it.
func (b *EventBus) Unsubscribe(name string, ch chan ContainerEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()

	subs := b.subscribers[name]
	for i, s := range subs {
		if s == ch {
			b.subscribers[name] = append(subs[:i], subs[i+1:]...)
			close(ch)
			return
		}
	}
}

// Publish sends an event to all subscribers matching the container name
// and to all wildcard ("") subscribers. Non-blocking — drops events if
// subscriber channel is full.
func (b *EventBus) Publish(event ContainerEvent) {
	if event.Timestamp == 0 {
		event.Timestamp = time.Now().Unix()
	}

	b.mu.RLock()
	defer b.mu.RUnlock()

	// Send to name-specific subscribers.
	for _, ch := range b.subscribers[event.Name] {
		select {
		case ch <- event:
		default: // drop if full
		}
	}

	// Send to wildcard subscribers.
	if event.Name != "" {
		for _, ch := range b.subscribers[""] {
			select {
			case ch <- event:
			default:
			}
		}
	}
}
