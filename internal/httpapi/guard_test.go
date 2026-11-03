package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHostGuard(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	h := hostGuard(ok)

	tests := []struct {
		name    string
		method  string
		host    string
		headers map[string]string
		body    string
		want    int
	}{
		{"loopback GET", "GET", "127.0.0.1:8080", nil, "", 204},
		{"localhost no port", "GET", "localhost", nil, "", 204},
		{"ipv6 loopback", "GET", "[::1]:8080", nil, "", 204},
		{"rebinding host", "GET", "evil.example:8080", nil, "", 403},
		{"rebinding lookalike", "GET", "localhost.evil.example", nil, "", 403},
		{"POST json same origin", "POST", "127.0.0.1:8080", map[string]string{"Origin": "http://127.0.0.1:8080", "Content-Type": "application/json"}, "{}", 204},
		{"POST via vite dev origin", "POST", "localhost:5173", map[string]string{"Origin": "http://localhost:5173", "Content-Type": "application/json"}, "{}", 204},
		{"POST foreign origin", "POST", "127.0.0.1:8080", map[string]string{"Origin": "https://evil.example", "Content-Type": "application/json"}, "{}", 403},
		{"POST null origin", "POST", "127.0.0.1:8080", map[string]string{"Origin": "null", "Content-Type": "application/json"}, "{}", 403},
		{"POST cross-site fetch metadata", "POST", "127.0.0.1:8080", map[string]string{"Sec-Fetch-Site": "cross-site", "Content-Type": "application/json"}, "{}", 403},
		{"POST text/plain body", "POST", "127.0.0.1:8080", map[string]string{"Content-Type": "text/plain"}, "{}", 415},
		{"POST json with charset", "POST", "127.0.0.1:8080", map[string]string{"Content-Type": "application/json; charset=utf-8"}, "{}", 204},
		{"POST empty body (cancel)", "POST", "127.0.0.1:8080", nil, "", 204},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/api/x", strings.NewReader(tt.body))
			req.Host = tt.host
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)
			if rr.Code != tt.want {
				t.Fatalf("status = %d, want %d (%s)", rr.Code, tt.want, rr.Body.String())
			}
		})
	}
}

func TestValidateJobOptions(t *testing.T) {
	bad := []createJobRequest{
		{Language: "--help"},
		{Language: "english-us"},
		{MaxLen: -1},
		{MaxLen: 5000},
		{MaxContext: -2},
		{MaxContext: 99999},
	}
	for _, r := range bad {
		if err := validateJobOptions(r); err == nil {
			t.Errorf("validateJobOptions(%+v) = nil, want error", r)
		}
	}
	good := []createJobRequest{{}, {Language: "auto"}, {Language: "ja", MaxLen: 40, MaxContext: -1}}
	for _, r := range good {
		if err := validateJobOptions(r); err != nil {
			t.Errorf("validateJobOptions(%+v) = %v, want nil", r, err)
		}
	}
}

func TestUnknownAPIRouteIsJSON404(t *testing.T) {
	mux := http.NewServeMux()
	(&Server{}).RegisterRoutes(mux)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest("GET", "/api/typo", nil))
	if rr.Code != 404 || !strings.Contains(rr.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("got %d %q, want JSON 404", rr.Code, rr.Header().Get("Content-Type"))
	}
}
