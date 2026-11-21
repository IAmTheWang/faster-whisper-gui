package httpapi

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Edge cases layered on top of TestHostGuard (guard_test.go), which covers the
// common allow/deny matrix. Here: Host spoofing tricks, non-POST state-changing
// methods, Origin parsing tricks, Sec-Fetch-Site values, Content-Type and
// unknown-length body handling, and the shape of the rejection itself.

func guardEdgeServe(h http.Handler, method, host string, headers map[string]string, body io.Reader) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/api/x", body)
	req.Host = host
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func TestHostGuard_EdgeCases(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	h := hostGuard(ok)
	const loop = "127.0.0.1:8080"
	with := func(extra map[string]string) map[string]string {
		m := map[string]string{"Content-Type": "application/json"}
		for k, v := range extra {
			m[k] = v
		}
		return m
	}

	tests := []struct {
		name    string
		method  string
		host    string
		headers map[string]string
		body    string
		want    int
	}{
		// Host header
		{"upper-case localhost", "GET", "LOCALHOST:8080", nil, "", 204},
		{"ipv6 loopback without port", "GET", "[::1]", nil, "", 204},
		{"empty Host (HTTP/1.0 style)", "GET", "", nil, "", 403},
		{"loopback as subdomain prefix", "GET", "127.0.0.1.evil.example", nil, "", 403},
		{"loopback as userinfo", "GET", "localhost@evil.example", nil, "", 403},
		{"loopback as suffix after colon", "GET", "evil.example:localhost", nil, "", 403},
		{"other loopback ipv6", "GET", "[::2]:8080", nil, "", 403},
		{"other 127/8 address is not allowlisted", "GET", "127.0.0.2:8080", nil, "", 403},
		{"public ip", "GET", "8.8.8.8", nil, "", 403},
		{"zero address", "GET", "0.0.0.0:8080", nil, "", 403},

		// Safe methods skip the Origin / Content-Type checks entirely.
		{"GET with foreign Origin", "GET", loop, map[string]string{"Origin": "https://evil.example"}, "", 204},
		{"HEAD with foreign Origin", "HEAD", loop, map[string]string{"Origin": "https://evil.example"}, "", 204},
		{"OPTIONS with foreign Origin", "OPTIONS", loop, map[string]string{"Origin": "https://evil.example"}, "", 204},
		{"HEAD still needs a loopback Host", "HEAD", "evil.example", nil, "", 403},

		// Every state-changing method is guarded, not only POST.
		{"PUT foreign origin", "PUT", loop, with(map[string]string{"Origin": "https://evil.example"}), "{}", 403},
		{"PUT same origin", "PUT", loop, with(map[string]string{"Origin": "http://127.0.0.1:8080"}), "{}", 204},
		{"PUT text/plain", "PUT", loop, map[string]string{"Content-Type": "text/plain"}, "{}", 415},
		{"DELETE foreign origin", "DELETE", loop, map[string]string{"Origin": "https://evil.example"}, "", 403},
		{"PATCH cross-site fetch metadata", "PATCH", loop, map[string]string{"Sec-Fetch-Site": "cross-site"}, "", 403},
		{"DELETE no body no origin", "DELETE", loop, nil, "", 204},

		// Origin parsing tricks.
		{"origin with ipv6 loopback", "POST", loop, with(map[string]string{"Origin": "http://[::1]:5173"}), "{}", 204},
		{"origin loopback as subdomain prefix", "POST", loop, with(map[string]string{"Origin": "http://127.0.0.1.evil.example"}), "{}", 403},
		{"origin loopback as userinfo", "POST", loop, with(map[string]string{"Origin": "http://localhost@evil.example"}), "{}", 403},
		{"origin lookalike host", "POST", loop, with(map[string]string{"Origin": "http://localhost.evil.example:8080"}), "{}", 403},
		{"origin unparsable", "POST", loop, with(map[string]string{"Origin": "://not a url"}), "{}", 403},
		{"origin browser extension", "POST", loop, with(map[string]string{"Origin": "chrome-extension://abcdef"}), "{}", 403},
		{"origin https loopback is fine", "POST", loop, with(map[string]string{"Origin": "https://localhost:8443"}), "{}", 204},

		// Sec-Fetch-Site: only "cross-site" is refused.
		{"fetch metadata same-origin", "POST", loop, with(map[string]string{"Sec-Fetch-Site": "same-origin"}), "{}", 204},
		{"fetch metadata same-site", "POST", loop, with(map[string]string{"Sec-Fetch-Site": "same-site"}), "{}", 204},
		{"fetch metadata none (user-initiated)", "POST", loop, with(map[string]string{"Sec-Fetch-Site": "none"}), "{}", 204},
		{"fetch metadata cross-site beats a loopback Origin", "POST", loop, with(map[string]string{"Origin": "http://localhost:5173", "Sec-Fetch-Site": "cross-site"}), "{}", 403},

		// Content-Type handling.
		{"upper-case JSON media type", "POST", loop, map[string]string{"Content-Type": "APPLICATION/JSON"}, "{}", 204},
		{"charset without space", "POST", loop, map[string]string{"Content-Type": "application/json;charset=utf-8"}, "{}", 204},
		{"json-patch is not json", "POST", loop, map[string]string{"Content-Type": "application/json-patch+json"}, "{}", 415},
		{"vendor +json is not json", "POST", loop, map[string]string{"Content-Type": "application/vnd.api+json"}, "{}", 415},
		{"form-urlencoded (the CSRF classic)", "POST", loop, map[string]string{"Content-Type": "application/x-www-form-urlencoded"}, "a=b", 415},
		{"multipart form", "POST", loop, map[string]string{"Content-Type": "multipart/form-data; boundary=x"}, "x", 415},
		{"body with no Content-Type", "POST", loop, nil, "{}", 415},
		{"malformed Content-Type", "POST", loop, map[string]string{"Content-Type": "application/json; =bad"}, "{}", 415},
		{"empty body ignores Content-Type", "POST", loop, map[string]string{"Content-Type": "text/plain"}, "", 204},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body io.Reader
			if tt.body != "" {
				body = strings.NewReader(tt.body)
			}
			rr := guardEdgeServe(h, tt.method, tt.host, tt.headers, body)
			if rr.Code != tt.want {
				t.Fatalf("status = %d, want %d (%s)", rr.Code, tt.want, strings.TrimSpace(rr.Body.String()))
			}
		})
	}
}

// A body of unknown length (chunked transfer: ContentLength == -1) is still a
// body, so it must still be JSON.
func TestHostGuard_UnknownLengthBodyMustBeJSON(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	h := hostGuard(ok)
	chunked := func(s string) io.Reader { return io.MultiReader(strings.NewReader(s)) } // hides the length

	req := httptest.NewRequest("POST", "/api/x", chunked("{}"))
	if req.ContentLength != -1 {
		t.Fatalf("test setup: ContentLength = %d, want -1", req.ContentLength)
	}
	req.Host = "127.0.0.1:8080"

	req.Header.Set("Content-Type", "text/plain")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnsupportedMediaType {
		t.Errorf("chunked text/plain = %d, want 415", rr.Code)
	}

	req = httptest.NewRequest("POST", "/api/x", chunked("{}"))
	req.Host = "127.0.0.1:8080"
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Errorf("chunked application/json = %d, want 204", rr.Code)
	}
}

// Rejections are JSON errors, and the wrapped handler must not run at all.
func TestHostGuard_RejectionShape(t *testing.T) {
	called := false
	h := hostGuard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))

	cases := []struct {
		name    string
		host    string
		headers map[string]string
		want    int
	}{
		{"foreign host", "evil.example", nil, http.StatusForbidden},
		{"foreign origin", "127.0.0.1:8080", map[string]string{"Origin": "https://evil.example", "Content-Type": "application/json"}, http.StatusForbidden},
		{"cross-site", "127.0.0.1:8080", map[string]string{"Sec-Fetch-Site": "cross-site", "Content-Type": "application/json"}, http.StatusForbidden},
		{"wrong media type", "127.0.0.1:8080", map[string]string{"Content-Type": "text/plain"}, http.StatusUnsupportedMediaType},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			called = false
			rr := guardEdgeServe(h, "POST", c.host, c.headers, strings.NewReader("{}"))
			if called {
				t.Error("the protected handler ran despite the rejection")
			}
			if rr.Code != c.want {
				t.Fatalf("status = %d, want %d", rr.Code, c.want)
			}
			msg := apiTestErrorBody(t, rr, c.want)
			if !strings.Contains(msg, "forbidden") && c.want == http.StatusForbidden {
				t.Errorf("error = %q, want it to say forbidden", msg)
			}
			if c.want == http.StatusUnsupportedMediaType && !strings.Contains(msg, "application/json") {
				t.Errorf("error = %q, want it to name the required media type", msg)
			}
		})
	}
}

func TestIsLoopbackHost(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"localhost", true},
		{"LocalHost", true},
		{"localhost:5173", true},
		{"127.0.0.1", true},
		{"127.0.0.1:0", true},
		{"[::1]", true},
		{"[::1]:8080", true},
		{"::1", true}, // bare ipv6 without brackets or port
		{"", false},
		{":8080", false},
		{"localhost.", false}, // trailing-dot FQDN form is not on the allowlist
		{"localhost.localdomain", false},
		{"127.0.0.1.nip.io", false},
		{"127.1", false},
		{"0x7f.0.0.1", false},
		{"2130706433", false}, // decimal form of 127.0.0.1
		{"[::ffff:127.0.0.1]", false},
		{"example.com:127", false},
	}
	for _, tt := range tests {
		if got := isLoopbackHost(tt.in); got != tt.want {
			t.Errorf("isLoopbackHost(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}
