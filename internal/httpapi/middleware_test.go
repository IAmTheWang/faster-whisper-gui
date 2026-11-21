package httpapi

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestWriteJSON(t *testing.T) {
	rr := httptest.NewRecorder()
	writeJSON(rr, http.StatusCreated, map[string]any{"a": 1, "b": []string{"x"}})

	if rr.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	if got, want := rr.Body.String(), `{"a":1,"b":["x"]}`+"\n"; got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
}

func TestWriteErrorMsgAndWriteError(t *testing.T) {
	t.Run("writeErrorMsg", func(t *testing.T) {
		rr := httptest.NewRecorder()
		writeErrorMsg(rr, http.StatusBadRequest, "bad thing")
		if rr.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", rr.Code)
		}
		if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q", ct)
		}
		var e errorResponse
		apiTestDecode(t, rr, &e)
		if e.Error != "bad thing" {
			t.Errorf("error = %q, want %q", e.Error, "bad thing")
		}
		if got, want := rr.Body.String(), `{"error":"bad thing"}`+"\n"; got != want {
			t.Errorf("body = %q, want %q", got, want)
		}
	})

	t.Run("writeError uses err.Error()", func(t *testing.T) {
		rr := httptest.NewRecorder()
		writeError(rr, http.StatusInternalServerError, errors.New("disk on fire"))
		if rr.Code != http.StatusInternalServerError {
			t.Errorf("status = %d, want 500", rr.Code)
		}
		var e errorResponse
		apiTestDecode(t, rr, &e)
		if e.Error != "disk on fire" {
			t.Errorf("error = %q", e.Error)
		}
	})

	// Messages embed filesystem paths and user input; the body must round-trip
	// exactly and never carry raw markup a sniffing browser could act on.
	t.Run("markup in the message is escaped and round-trips", func(t *testing.T) {
		msg := `bad <script>alert("x")</script> & friends`
		rr := httptest.NewRecorder()
		writeErrorMsg(rr, http.StatusBadRequest, msg)
		if strings.ContainsAny(rr.Body.String(), "<>") {
			t.Errorf("raw angle brackets in JSON body: %s", rr.Body.String())
		}
		var e errorResponse
		apiTestDecode(t, rr, &e)
		if e.Error != msg {
			t.Errorf("round-trip = %q, want %q", e.Error, msg)
		}
	})
}

// mwCaptureSlog redirects the default logger into the returned buffer.
func mwCaptureSlog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	orig := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(orig) })
	return &buf
}

func TestRecoverMiddleware(t *testing.T) {
	logs := mwCaptureSlog(t)
	const secret = "super-secret-internal-detail"

	mux := http.NewServeMux()
	mux.HandleFunc("/panic-string", func(http.ResponseWriter, *http.Request) { panic(secret) })
	mux.HandleFunc("/panic-error", func(http.ResponseWriter, *http.Request) { panic(errors.New(secret)) })
	mux.HandleFunc("/panic-nil-deref", func(http.ResponseWriter, *http.Request) {
		var m map[string]int
		m["x"] = 1 // assignment to a nil map panics
	})
	mux.HandleFunc("/ok", func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "fine") })

	// A real server, so "the server survives" is literal: after each panic the
	// same listener keeps answering.
	ts := httptest.NewServer(recoverMiddleware(mux))
	defer ts.Close()

	get := func(path string) (int, string) {
		t.Helper()
		resp, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v (did the connection die with the panic?)", path, err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}

	for _, path := range []string{"/panic-string", "/panic-error", "/panic-nil-deref"} {
		code, body := get(path)
		if code != http.StatusInternalServerError {
			t.Errorf("GET %s = %d, want 500", path, code)
		}
		if strings.Contains(body, secret) {
			t.Errorf("GET %s leaked the panic value to the client: %q", path, body)
		}
		if !strings.Contains(body, "internal server error") {
			t.Errorf("GET %s body = %q, want a generic internal-server-error message", path, body)
		}
		// Still alive afterwards.
		if code, body := get("/ok"); code != http.StatusOK || body != "fine" {
			t.Fatalf("after %s: GET /ok = %d %q, want 200 fine", path, code, body)
		}
	}

	if out := logs.String(); !strings.Contains(out, "panic recovered") || !strings.Contains(out, secret) {
		t.Errorf("the panic should be logged server-side, logs = %q", out)
	}
}

func TestRecoverMiddleware_PassesThroughWhenNoPanic(t *testing.T) {
	h := recoverMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		_, _ = io.WriteString(w, "short and stout")
	}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/", nil))
	if rr.Code != http.StatusTeapot || rr.Body.String() != "short and stout" {
		t.Fatalf("got %d %q, want the handler's own response", rr.Code, rr.Body.String())
	}
}

// A panic deep inside the full stack must still produce a 500 that carries the
// security headers, since those are set before the handler runs.
func TestWithMiddleware_PanicGets500WithSecurityHeaders(t *testing.T) {
	mwCaptureSlog(t)
	h := WithMiddleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") }))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "http://127.0.0.1:8080/api/x", nil))
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rr.Code)
	}
	if got := rr.Header().Get("X-Frame-Options"); got != "DENY" {
		t.Errorf("X-Frame-Options = %q on the 500 response, want DENY", got)
	}
}

func TestWithMiddleware_RejectedRequestsStillGetSecurityHeaders(t *testing.T) {
	h := WithMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }))
	req := httptest.NewRequest("GET", "http://evil.example/api/x", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 for a foreign Host", rr.Code)
	}
	for k, want := range map[string]string{"X-Frame-Options": "DENY", "X-Content-Type-Options": "nosniff", "Referrer-Policy": "no-referrer"} {
		if got := rr.Header().Get(k); got != want {
			t.Errorf("%s = %q on the 403, want %q", k, got, want)
		}
	}
}

// bareResponseWriter is a ResponseWriter with no Flush/SetWriteDeadline, like some
// real-world wrappers.
type bareResponseWriter struct {
	header http.Header
	code   int
	body   bytes.Buffer
}

func (p *bareResponseWriter) Header() http.Header         { return p.header }
func (p *bareResponseWriter) Write(b []byte) (int, error) { return p.body.Write(b) }
func (p *bareResponseWriter) WriteHeader(code int)        { p.code = code }

func TestStatusWriter(t *testing.T) {
	t.Run("tracks the status and forwards it", func(t *testing.T) {
		rr := httptest.NewRecorder()
		sw := &statusWriter{ResponseWriter: rr, status: http.StatusOK}
		sw.WriteHeader(http.StatusNotFound)
		if sw.status != http.StatusNotFound || rr.Code != http.StatusNotFound {
			t.Errorf("sw.status = %d, underlying = %d, want 404 for both", sw.status, rr.Code)
		}
	})

	t.Run("Unwrap returns the underlying writer", func(t *testing.T) {
		rr := httptest.NewRecorder()
		sw := &statusWriter{ResponseWriter: rr}
		if sw.Unwrap() != http.ResponseWriter(rr) {
			t.Error("Unwrap did not return the wrapped writer")
		}
	})

	t.Run("Flush reaches a flushing writer", func(t *testing.T) {
		rr := httptest.NewRecorder()
		sw := &statusWriter{ResponseWriter: rr}
		sw.Flush()
		if !rr.Flushed {
			t.Error("underlying writer was not flushed")
		}
	})

	t.Run("Flush on a non-flusher is a harmless no-op", func(t *testing.T) {
		sw := &statusWriter{ResponseWriter: &bareResponseWriter{header: http.Header{}}}
		sw.Flush() // must not panic
	})

	t.Run("ResponseController sees through the wrapper", func(t *testing.T) {
		rr := httptest.NewRecorder()
		sw := &statusWriter{ResponseWriter: rr}
		if err := http.NewResponseController(sw).Flush(); err != nil {
			t.Fatalf("ResponseController.Flush through statusWriter: %v", err)
		}
		if !rr.Flushed {
			t.Error("underlying writer was not flushed")
		}
	})

	// The SSE handler clears its write deadline through the controller; that
	// call has no shim on statusWriter and must reach the real writer via Unwrap.
	t.Run("ResponseController deadlines reach the underlying writer", func(t *testing.T) {
		dw := &deadlineResponseWriter{bareResponseWriter: bareResponseWriter{header: http.Header{}}}
		sw := &statusWriter{ResponseWriter: dw}
		want := time.Now().Add(time.Hour)
		if err := http.NewResponseController(sw).SetWriteDeadline(want); err != nil {
			t.Fatalf("SetWriteDeadline through statusWriter: %v", err)
		}
		if !dw.deadline.Equal(want) {
			t.Errorf("underlying deadline = %v, want %v", dw.deadline, want)
		}
	})

	t.Run("ResponseController reports ErrNotSupported rather than panicking", func(t *testing.T) {
		sw := &statusWriter{ResponseWriter: &bareResponseWriter{header: http.Header{}}}
		err := http.NewResponseController(sw).SetWriteDeadline(time.Time{})
		if !errors.Is(err, http.ErrNotSupported) {
			t.Errorf("SetWriteDeadline error = %v, want ErrNotSupported", err)
		}
	})
}

// deadlineResponseWriter adds SetWriteDeadline, which http.ResponseController looks for.
type deadlineResponseWriter struct {
	bareResponseWriter
	deadline time.Time
}

func (d *deadlineResponseWriter) SetWriteDeadline(t time.Time) error {
	d.deadline = t
	return nil
}

// The SSE handler depends on Flush working when served through WithMiddleware.
func TestWithMiddleware_FlushSucceedsThroughWrappers(t *testing.T) {
	var flushErr error
	h := WithMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "first")
		flushErr = http.NewResponseController(w).Flush()
	}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "http://127.0.0.1:8080/api/stream", nil))
	if flushErr != nil {
		t.Fatalf("Flush through WithMiddleware: %v", flushErr)
	}
	if !rr.Flushed {
		t.Error("the flush never reached the underlying writer")
	}
	if rr.Body.String() != "first" {
		t.Errorf("body = %q", rr.Body.String())
	}
}

func TestLoggingMiddleware(t *testing.T) {
	logs := mwCaptureSlog(t)
	h := loggingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/api/jobs/abc/events?token=TOPSECRET", nil))

	out := logs.String()
	for _, want := range []string{"method=GET", "path=/api/jobs/abc/events", "status=418"} {
		if !strings.Contains(out, want) {
			t.Errorf("log %q missing %q", out, want)
		}
	}
	// token.go relies on the logger recording only the path, never the query.
	if strings.Contains(out, "TOPSECRET") {
		t.Errorf("the query string (and its token) leaked into the log: %q", out)
	}
}

func TestLoggingMiddleware_DefaultStatusIs200(t *testing.T) {
	logs := mwCaptureSlog(t)
	h := loggingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "no explicit WriteHeader")
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/x", nil))
	if !strings.Contains(logs.String(), "status=200") {
		t.Errorf("log = %q, want status=200", logs.String())
	}
}
