// Package sseutil implements a minimal per-job Server-Sent Events broker:
// each job gets its own topic, and one or more HTTP handlers can subscribe
// to receive that job's events as they're published.
package sseutil

import "sync"

// Event is a single SSE message: Type becomes the "event:" field, Data is
// marshaled to JSON for the "data:" field by the HTTP handler.
type Event struct {
	Type string
	Data any
}

// Broker fans out events published for a job ID to every currently
// subscribed channel. It implements internal/job.EventPublisher.
type Broker struct {
	mu   sync.Mutex
	subs map[string]map[chan Event]struct{}
}

func NewBroker() *Broker {
	return &Broker{subs: make(map[string]map[chan Event]struct{})}
}

// Subscribe registers a new listener for jobID's events. The caller must
// call the returned unsubscribe func (typically via defer) once it's done
// reading, or the channel will leak.
func (b *Broker) Subscribe(jobID string) (ch chan Event, unsubscribe func()) {
	ch = make(chan Event, 32)

	b.mu.Lock()
	if b.subs[jobID] == nil {
		b.subs[jobID] = make(map[chan Event]struct{})
	}
	b.subs[jobID][ch] = struct{}{}
	b.mu.Unlock()

	unsubscribe = func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if set, ok := b.subs[jobID]; ok {
			delete(set, ch)
			if len(set) == 0 {
				delete(b.subs, jobID)
			}
		}
		close(ch)
	}
	return ch, unsubscribe
}

// Publish sends an event to every current subscriber of jobID. Slow
// subscribers that haven't drained their channel simply miss the event
// rather than blocking the job pipeline — SSE progress is best-effort, and
// GET /api/jobs/{id} remains available for the authoritative current state.
func (b *Broker) Publish(jobID string, eventType string, data any) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs[jobID] {
		select {
		case ch <- Event{Type: eventType, Data: data}:
		default:
		}
	}
}
