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
	initial, ok := s.Store.Get(id)
	if !ok {
		writeErrorMsg(w, http.StatusNotFound, "job not found")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Time{})

	ch, unsubscribe := s.Broker.Subscribe(id)
	defer unsubscribe()

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

func writeSSE(w io.Writer, rc *http.ResponseController, ev sseutil.Event) bool {
	payload, err := json.Marshal(ev.Data)
	if err != nil {
		return false
	}
	if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, payload); err != nil {
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
