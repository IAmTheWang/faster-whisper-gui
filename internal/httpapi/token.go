package httpapi

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// TokenHeader carries the API token on fetch requests.
const TokenHeader = "X-Api-Token"

// WithToken rejects /api/ requests that don't present token (401). Anything
// else (the embedded frontend's static files) passes through untouched, since
// the UI is what hands the token to the browser in the first place.
//
// The token is read from the X-Api-Token header; EventSource cannot set
// headers, so the SSE endpoint (GET .../events) also accepts ?token=. The
// request logger records only r.URL.Path, never the query, so the token does
// not reach the logs. An empty token disables the check (tests).
func WithToken(next http.Handler, token string) http.Handler {
	if token == "" {
		return next
	}
	want := []byte(token)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			got := r.Header.Get(TokenHeader)
			if got == "" && r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/events") {
				got = r.URL.Query().Get("token")
			}
			if subtle.ConstantTimeCompare([]byte(got), want) != 1 {
				writeErrorMsg(w, http.StatusUnauthorized, "missing or invalid API token")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
