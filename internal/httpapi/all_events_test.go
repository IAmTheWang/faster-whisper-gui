package httpapi

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"faster-whisper-gui/internal/job"
	"faster-whisper-gui/internal/sseutil"
)

// readFrame returns the next event name and decoded data from an SSE stream,
// skipping comment lines (": connected", heartbeats).
func readFrame(t *testing.T, br *bufio.Reader) (string, allEventsFrame) {
	t.Helper()
	var name string
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			t.Fatalf("reading SSE stream: %v", err)
		}
		line = strings.TrimSpace(line)
		if n, ok := strings.CutPrefix(line, "event: "); ok {
			name = n
		} else if d, ok := strings.CutPrefix(line, "data: "); ok {
			var f allEventsFrame
			if err := json.Unmarshal([]byte(d), &f); err != nil {
				t.Fatalf("bad frame %q: %v", d, err)
			}
			return name, f
		}
	}
}

func TestAllEvents_StreamsEveryJobWithItsID(t *testing.T) {
	broker := sseutil.NewBroker()
	s := NewServer(nil, job.NewStore(), nil, broker)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/events", s.handleAllEvents)
	ts := httptest.NewServer(WithMiddleware(mux))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type = %q", ct)
	}
	br := bufio.NewReader(resp.Body)

	// Headers (and ": connected") are flushed at once, so Get returned already;
	// publish until the handler — subscribed by now or very soon — forwards them.
	go func() {
		for i := 0; i < 100; i++ {
			broker.Publish("a", "progress", map[string]any{"stage": "transcribing", "percent": 10})
			broker.Publish("b", "done", map[string]string{"srtPath": "b.srt"})
			time.Sleep(20 * time.Millisecond)
		}
	}()

	seen := map[string]string{}
	for len(seen) < 2 {
		name, f := readFrame(t, br)
		seen[f.JobID] = name
	}
	if seen["a"] != "progress" || seen["b"] != "done" {
		t.Fatalf("events by job = %v", seen)
	}
}

func TestAllEvents_EndsOnCloseStreams(t *testing.T) {
	s := NewServer(nil, job.NewStore(), nil, sseutil.NewBroker())
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/events", s.handleAllEvents)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	s.CloseStreams()
	s.CloseStreams() // idempotent

	done := make(chan struct{})
	go func() {
		bufio.NewReader(resp.Body).ReadString('\x00') // returns when the server ends the stream
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("stream still open after CloseStreams")
	}
}

func TestBrokerSubscribeAllCarriesJobID(t *testing.T) {
	b := sseutil.NewBroker()
	ch, unsub := b.SubscribeAll()
	defer unsub()
	b.Publish("j1", "progress", 1)
	b.Publish("j2", "done", 2)
	for _, want := range []string{"j1", "j2"} {
		select {
		case ev := <-ch:
			if ev.JobID != want {
				t.Fatalf("JobID = %q, want %q", ev.JobID, want)
			}
		case <-time.After(time.Second):
			t.Fatal("event not delivered")
		}
	}
}
