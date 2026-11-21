package httpapi

import (
	"log/slog"
	"net/http"
	"time"
)

// WithMiddleware wraps h with request logging and panic recovery. Apply it
// once around the whole top-level mux (API routes + embedded static files),
// not just the API routes, so a panic anywhere still gets a clean 500
// instead of taking the server down. Recovery sits inside logging, so a
// panicking request is still logged (as a 500).
func WithMiddleware(h http.Handler) http.Handler {
	return loggingMiddleware(recoverMiddleware(securityHeaders(hostGuard(h))))
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		slog.Info("http", "method", r.Method, "path", r.URL.Path, "status", sw.status, "duration", time.Since(start))
	})
}

func recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				// net/http's own convention for "abort this response quietly".
				if rec == http.ErrAbortHandler {
					panic(rec)
				}
				slog.Error("panic recovered", "error", rec, "path", r.URL.Path)
				if sw, ok := w.(*statusWriter); ok {
					if sw.wrote {
						return // headers are out already; a 500 can no longer be sent
					}
					sw.status = http.StatusInternalServerError
				}
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
	wrote  bool // the status line has been sent
}

func (sw *statusWriter) WriteHeader(code int) {
	sw.status = code
	sw.wrote = true
	sw.ResponseWriter.WriteHeader(code)
}

func (sw *statusWriter) Write(b []byte) (int, error) {
	sw.wrote = true // an implicit 200 counts
	return sw.ResponseWriter.Write(b)
}

// Unwrap lets http.NewResponseController reach the underlying writer's
// Flush/SetWriteDeadline; without it the SSE handler's Flush fails with
// ErrNotSupported when served through this middleware.
func (sw *statusWriter) Unwrap() http.ResponseWriter { return sw.ResponseWriter }

// Flush keeps plain w.(http.Flusher) assertions working too.
func (sw *statusWriter) Flush() {
	if f, ok := sw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// contentSecurityPolicy locks the UI down to its own origin: no inline or
// third-party script, no framing, no <base> hijacking. Inline styles stay
// allowed because components set style attributes.
const contentSecurityPolicy = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'"

// securityHeaders hardens every response (UI and API) against framing,
// MIME sniffing and referrer leakage.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Content-Security-Policy", contentSecurityPolicy)
		next.ServeHTTP(w, r)
	})
}
