package httpapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"faster-whisper-gui/internal/job"
	"faster-whisper-gui/internal/sseutil"
)

const sseHeartbeatInterval = 15 * time.Second

// handleJobEvents streams a single job's lifecycle as Server-Sent Events.
//
// It deliberately does not rely on any server-wide WriteTimeout (there
// isn't one — see cmd/server/main.go) and additionally clears any write
// deadline via http.ResponseController so a long transcription can't have
// its SSE connection cut mid-stream. A periodic heartbeat comment keeps the
// connection from being treated as idle while progress is sparse.
func (s *Server) handleJobEvents(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := s.Store.Get(id); !ok {
		writeErrorMsg(w, http.StatusNotFound, "job not found")
		return
	}

	// Subscribe before taking the snapshot: a job finishing in between would
	// otherwise be missed by both the snapshot and the subscription.
	ch, unsubscribe := s.Broker.Subscribe(id)
	defer unsubscribe()
	initial, _ := s.Store.Get(id)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Time{})

	if !writeSSE(w, rc, sseEventFromJob(initial)) {
		return
	}
	if initial.Status.IsTerminal() {
		return
	}

	heartbeat := time.NewTicker(sseHeartbeatInterval)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return

		case <-heartbeat.C:
			// Publish is best-effort, so a terminal event can be dropped for a
			// slow client; the store is the source of truth.
			if cur, ok := s.Store.Get(id); ok && cur.Status.IsTerminal() {
				writeSSE(w, rc, sseEventFromJob(cur))
				return
			}
			if _, err := io.WriteString(w, ": heartbeat\n\n"); err != nil {
				return
			}
			if err := rc.Flush(); err != nil {
				return
			}

		case ev, ok := <-ch:
			if !ok {
				return
			}
			if !writeSSE(w, rc, ev) {
				return
			}
			if ev.Type == "done" || ev.Type == "error" || ev.Type == "canceled" {
				return
			}
		}
	}
}

// allEventsFrame is the data payload of one event on the all-jobs stream: the
// ordinary per-job payload plus the job it belongs to.
type allEventsFrame struct {
	JobID string `json:"jobId"`
	Data  any    `json:"data"`
}

// handleAllEvents streams the events of every job on one connection, so the UI
// needs a single EventSource however many Progress tabs are open (browsers cap
// concurrent HTTP/1.1 connections per host at about six, and each per-job
// stream would hold one for the life of its job).
//
// Unlike the per-job endpoint it sends no snapshot: there is no single "current
// state" to replay. A client resyncs each job it cares about with
// GET /api/jobs/{id} when the stream opens (and on every reconnect) — the
// headers are flushed immediately so "open" fires at once — and Publish evicts
// to keep terminal events, so a finished job is not missed in between.
func (s *Server) handleAllEvents(w http.ResponseWriter, r *http.Request) {
	ch, unsubscribe := s.Broker.SubscribeAll()
	defer unsubscribe()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Time{})
	if _, err := io.WriteString(w, ": connected\n\n"); err != nil || rc.Flush() != nil {
		return
	}

	heartbeat := time.NewTicker(sseHeartbeatInterval)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-s.streamsDone:
			return
		case <-heartbeat.C:
			if _, err := io.WriteString(w, ": heartbeat\n\n"); err != nil {
				return
			}
			if rc.Flush() != nil {
				return
			}
		case ev, ok := <-ch:
			if !ok {
				return
			}
			if !writeSSEData(w, rc, ev.Type, allEventsFrame{JobID: ev.JobID, Data: ev.Data}) {
				return
			}
		}
	}
}

func writeSSE(w io.Writer, rc *http.ResponseController, ev sseutil.Event) bool {
	return writeSSEData(w, rc, ev.Type, ev.Data)
}

func writeSSEData(w io.Writer, rc *http.ResponseController, eventType string, data any) bool {
	payload, err := json.Marshal(data)
	if err != nil {
		return false
	}
	if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, payload); err != nil {
		return false
	}
	return rc.Flush() == nil
}

// sseEventFromJob synthesizes the event a freshly-subscribed client should
// see immediately, reflecting the job's current state rather than making it
// wait for the next state change (which may never come, if the job already
// finished before this connection was opened).
func sseEventFromJob(j job.Job) sseutil.Event {
	switch j.Status {
	case job.StatusDone:
		return sseutil.Event{Type: "done", Data: map[string]string{"srtPath": j.SRTPath}}
	case job.StatusFailed:
		return sseutil.Event{Type: "error", Data: map[string]string{"message": j.Error}}
	case job.StatusCanceled:
		return sseutil.Event{Type: "canceled", Data: map[string]string{}}
	default:
		return sseutil.Event{Type: "progress", Data: map[string]any{
			"stage":   string(j.Status),
			"percent": j.Percent,
			"message": j.Message,
		}}
	}
}
