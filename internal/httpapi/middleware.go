package httpapi

import (
	"log/slog"
	"net/http"
	"time"
)

// WithMiddleware wraps h with request logging and panic recovery. Apply it
// once around the whole top-level mux (API routes + embedded static files),
// not just the API routes, so a panic anywhere still gets a clean 500
// instead of taking the server down.
func WithMiddleware(h http.Handler) http.Handler {
	return recoverMiddleware(loggingMiddleware(hostGuard(h)))
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
				slog.Error("panic recovered", "error", rec, "path", r.URL.Path)
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (sw *statusWriter) WriteHeader(code int) {
	sw.status = code
	sw.ResponseWriter.WriteHeader(code)
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
