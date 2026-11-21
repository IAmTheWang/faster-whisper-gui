package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSecurityHeaders(t *testing.T) {
	h := WithMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "http://127.0.0.1:8080/", nil))
	for k, want := range map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "no-referrer",
	} {
		if got := rr.Header().Get(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
	if csp := rr.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "frame-ancestors 'none'") || !strings.Contains(csp, "script-src 'self'") {
		t.Errorf("CSP = %q", csp)
	}
}

func TestDecodeJSONIsStrict(t *testing.T) {
	type body struct {
		A string `json:"a"`
	}
	cases := []struct {
		name, in string
		ok       bool
	}{
		{"known field", `{"a":"x"}`, true},
		{"trailing whitespace", "{\"a\":\"x\"}\n", true},
		{"unknown field", `{"a":"x","b":1}`, false},
		{"trailing data", `{"a":"x"}{"a":"y"}`, false},
		{"garbage after", `{"a":"x"} junk`, false},
	}
	for _, c := range cases {
		req := httptest.NewRequest("POST", "/", strings.NewReader(c.in))
		var b body
		err := decodeJSON(httptest.NewRecorder(), req, &b)
		if (err == nil) != c.ok {
			t.Errorf("%s: err = %v, want ok=%v", c.name, err, c.ok)
		}
	}
}
