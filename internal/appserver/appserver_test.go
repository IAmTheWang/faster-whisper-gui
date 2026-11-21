package appserver

import (
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func TestRequireLoopbackAddr(t *testing.T) {
	for _, ok := range []string{"127.0.0.1:8080", "localhost:8080", "[::1]:8080", "127.0.0.2:1"} {
		if err := requireLoopbackAddr(ok); err != nil {
			t.Errorf("requireLoopbackAddr(%q) = %v, want nil", ok, err)
		}
	}
	for _, bad := range []string{"0.0.0.0:8080", ":8080", "192.168.1.5:8080", "example.com:80", "nonsense"} {
		if err := requireLoopbackAddr(bad); err == nil {
			t.Errorf("requireLoopbackAddr(%q) = nil, want error", bad)
		}
	}
}

func TestSpaHandlerFallback(t *testing.T) {
	h := spaHandler(fstest.MapFS{
		"index.html":    {Data: []byte("<html>app</html>")},
		"assets/app.js": {Data: []byte("js")},
	}, "tok")
	for path, want := range map[string]int{
		"/":               200,
		"/assets/app.js":  200,
		"/some/route":     200, // extension-less: SPA fallback
		"/assets/gone.js": 404, // missing asset must not return index.html
	} {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest("GET", path, nil))
		if rr.Code != want {
			t.Errorf("GET %s = %d, want %d", path, rr.Code, want)
		}
	}
}

func TestIndexGetsAPIToken(t *testing.T) {
	h := spaHandler(fstest.MapFS{
		"index.html": {Data: []byte(`<meta name="api-token" content="__API_TOKEN__">`)},
	}, "abc123")
	for _, path := range []string{"/", "/index.html", "/some/route"} {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest("GET", path, nil))
		if got := rr.Body.String(); got != `<meta name="api-token" content="abc123">` {
			t.Errorf("GET %s body = %q", path, got)
		}
		if rr.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("GET %s must not be cacheable", path)
		}
	}
}
