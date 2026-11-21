package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWithToken(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	h := WithToken(ok, "s3cret")

	cases := []struct {
		name, method, url, header string
		want                      int
	}{
		{"api without token", "GET", "/api/jobs", "", 401},
		{"api wrong token", "GET", "/api/jobs", "nope", 401},
		{"api with header", "GET", "/api/jobs", "s3cret", 204},
		{"post with header", "POST", "/api/jobs", "s3cret", 204},
		{"post without token", "POST", "/api/jobs", "", 401},
		{"static needs no token", "GET", "/assets/app.js", "", 204},
		{"index needs no token", "GET", "/", "", 204},
		{"sse query token", "GET", "/api/jobs/abc/events?token=s3cret", "", 204},
		{"sse bad query token", "GET", "/api/jobs/abc/events?token=x", "", 401},
		{"query token not honoured elsewhere", "GET", "/api/jobs?token=s3cret", "", 401},
		{"query token not honoured on POST", "POST", "/api/jobs/abc/events?token=s3cret", "", 401},
	}
	for _, c := range cases {
		req := httptest.NewRequest(c.method, c.url, nil)
		if c.header != "" {
			req.Header.Set(TokenHeader, c.header)
		}
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != c.want {
			t.Errorf("%s: got %d, want %d", c.name, rr.Code, c.want)
		}
	}
}

func TestWithTokenEmptyDisablesCheck(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	rr := httptest.NewRecorder()
	WithToken(ok, "").ServeHTTP(rr, httptest.NewRequest("GET", "/api/jobs", nil))
	if rr.Code != 204 {
		t.Fatalf("got %d", rr.Code)
	}
}
