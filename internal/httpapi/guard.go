package httpapi

import (
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
)

// hostGuard defends the local-only API against DNS rebinding and cross-site
// requests. The server binds loopback, but a web page the user visits can
// still reach it: through a rebinding domain (same-origin from the browser's
// view, but with a foreign Host header) or via "simple" cross-origin POSTs
// that need no preflight. The API exposes the filesystem and lets settings
// point at executables, so both must be refused.
//
//   - Host must be a loopback name (any port — cmd/dev serves the UI from
//     Vite on another port and proxies /api here).
//   - State-changing requests must not carry a non-loopback Origin or a
//     cross-site Sec-Fetch-Site, and any request body must be JSON (which a
//     cross-origin form/fetch cannot send without a preflight).
func hostGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isLoopbackHost(r.Host) {
			writeErrorMsg(w, http.StatusForbidden, "forbidden: unexpected Host header")
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
			if origin := r.Header.Get("Origin"); origin != "" && origin != "null" {
				u, err := url.Parse(origin)
				if err != nil || !isLoopbackHost(u.Host) {
					writeErrorMsg(w, http.StatusForbidden, "forbidden: cross-origin request")
					return
				}
			} else if origin == "null" {
				writeErrorMsg(w, http.StatusForbidden, "forbidden: cross-origin request")
				return
			}
			if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
				writeErrorMsg(w, http.StatusForbidden, "forbidden: cross-site request")
				return
			}
			if r.ContentLength != 0 {
				mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
				if err != nil || mt != "application/json" {
					writeErrorMsg(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

// isLoopbackHost reports whether hostport (a Host header or URL host) names
// localhost, 127.0.0.1 or [::1], with or without a port.
func isLoopbackHost(hostport string) bool {
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	host = strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
	switch strings.ToLower(host) {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	return false
}
