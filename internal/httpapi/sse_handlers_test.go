package httpapi

import (
	"bufio"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"faster-whisper-gui/internal/job"
	"faster-whisper-gui/internal/sseutil"
)

// readEvent returns the next "event:" name from an SSE stream.
func readEvent(t *testing.T, br *bufio.Reader) string {
	t.Helper()
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			t.Fatalf("reading SSE stream: %v", err)
		}
		if name, ok := strings.CutPrefix(strings.TrimSpace(line), "event: "); ok {
			return name
		}
	}
}

// TestJobEvents_ThroughMiddleware guards against statusWriter hiding Flush:
// the stream must stay open past the first event when served via
// WithMiddleware, as in the real server.
func TestJobEvents_ThroughMiddleware(t *testing.T) {
	store := job.NewStore()
	store.Add(&job.Job{ID: "j1", Status: job.StatusTranscribing, CreatedAt: time.Now()})
	broker := sseutil.NewBroker()
	s := &Server{Store: store, Broker: broker}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/jobs/{id}/events", s.handleJobEvents)
	ts := httptest.NewServer(WithMiddleware(mux))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/jobs/j1/events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	br := bufio.NewReader(resp.Body)

	if got := readEvent(t, br); got != "progress" {
		t.Fatalf("first event = %q, want progress", got)
	}

	// Publish until the handler (already subscribed before the snapshot) forwards it.
	go func() {
		for i := 0; i < 50; i++ {
			broker.Publish("j1", "done", map[string]string{"srtPath": "x.srt"})
			time.Sleep(20 * time.Millisecond)
		}
	}()
	if got := readEvent(t, br); got != "done" {
		t.Fatalf("second event = %q, want done", got)
	}
}

func TestJobEvents_AlreadyTerminal(t *testing.T) {
	store := job.NewStore()
	store.Add(&job.Job{ID: "j1", Status: job.StatusDone, SRTPath: "x.srt", CreatedAt: time.Now()})
	s := &Server{Store: store, Broker: sseutil.NewBroker()}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/jobs/{id}/events", s.handleJobEvents)
	ts := httptest.NewServer(WithMiddleware(mux))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/jobs/j1/events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if got := readEvent(t, bufio.NewReader(resp.Body)); got != "done" {
		t.Fatalf("event = %q, want done", got)
	}
}
