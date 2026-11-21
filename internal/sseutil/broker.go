// Package sseutil implements a minimal Server-Sent Events broker: each job
// gets its own topic, one or more HTTP handlers can subscribe to a single
// job's events, and SubscribeAll gives one subscriber the events of every job.
package sseutil

import "sync"

// Event is a single SSE message: Type becomes the "event:" field, Data is
// marshaled to JSON for the "data:" field by the HTTP handler. JobID is set by
// Publish, so a SubscribeAll listener can tell which job an event belongs to.
type Event struct {
	JobID string
	Type  string
	Data  any
}

const (
	// allKey is the topic SubscribeAll registers under; job IDs are never empty.
	allKey = ""

	jobBuffer = 32
	// The all-jobs stream interleaves every job (only one runs at a time, but
	// queued ones cancel and finish too), so give it more room.
	allBuffer = 256
)

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
	return b.subscribe(jobID, jobBuffer)
}

// SubscribeAll registers a listener for the events of every job (each carries
// its JobID). One such stream replaces one connection per open job in the UI.
// The same unsubscribe rules as Subscribe apply.
func (b *Broker) SubscribeAll() (ch chan Event, unsubscribe func()) {
	return b.subscribe(allKey, allBuffer)
}

func (b *Broker) subscribe(key string, buffer int) (ch chan Event, unsubscribe func()) {
	ch = make(chan Event, buffer)

	b.mu.Lock()
	if b.subs[key] == nil {
		b.subs[key] = make(map[chan Event]struct{})
	}
	b.subs[key][ch] = struct{}{}
	b.mu.Unlock()

	var once sync.Once
	unsubscribe = func() {
		once.Do(func() {
			b.mu.Lock()
			defer b.mu.Unlock()
			if set, ok := b.subs[key]; ok {
				delete(set, ch)
				if len(set) == 0 {
					delete(b.subs, key)
				}
			}
			close(ch)
		})
	}
	return ch, unsubscribe
}

// Publish sends an event to every current subscriber of jobID and to every
// SubscribeAll listener. Slow subscribers that haven't drained their channel
// simply miss the event rather than blocking the job pipeline — SSE progress
// is best-effort, and GET /api/jobs/{id} remains available for the
// authoritative current state.
//
// Terminal events (done/error/canceled) are the exception: a client that
// misses one would wait for a state change that never comes, so when a
// subscriber's buffer is full the oldest buffered event is evicted to make
// room (progress events dominate, so that is almost always a progress event).
func (b *Broker) Publish(jobID string, eventType string, data any) {
	ev := Event{JobID: jobID, Type: eventType, Data: data}
	terminal := isTerminal(eventType)
	b.mu.Lock()
	defer b.mu.Unlock()
	deliver(b.subs[jobID], ev, terminal)
	deliver(b.subs[allKey], ev, terminal)
}

func deliver(set map[chan Event]struct{}, ev Event, terminal bool) {
	for ch := range set {
		select {
		case ch <- ev:
			continue
		default:
		}
		if !terminal {
			continue
		}
		// Publish holds b.mu, so no other publisher can refill the slot we free;
		// a concurrent reader can only make more room.
		select {
		case <-ch:
		default:
		}
		select {
		case ch <- ev:
		default:
		}
	}
}

func isTerminal(eventType string) bool {
	switch eventType {
	case "done", "error", "canceled":
		return true
	}
	return false
}
