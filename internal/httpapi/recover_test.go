package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRecoverKeepsHeadersAlreadySent(t *testing.T) {
	h := WithMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		panic("after headers")
	}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "http://127.0.0.1:8080/", nil))
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want the already-sent 202 (a second status line is impossible)", rr.Code)
	}
}

func TestRecoverSendsA500BeforeAnyOutput(t *testing.T) {
	h := WithMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { panic("early") }))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "http://127.0.0.1:8080/", nil))
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rr.Code)
	}
}

func TestRecoverRepanicsErrAbortHandler(t *testing.T) {
	h := recoverMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { panic(http.ErrAbortHandler) }))
	defer func() {
		if rec := recover(); rec != http.ErrAbortHandler {
			t.Fatalf("recovered %v, want http.ErrAbortHandler re-panicked", rec)
		}
	}()
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
	t.Fatal("expected a panic")
}
