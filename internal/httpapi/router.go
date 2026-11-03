package httpapi

import "net/http"

// RegisterRoutes registers every /api/* route onto mux. The caller
// (cmd/server/main.go) is responsible for mounting the embedded frontend
// alongside this and wrapping the combined mux with WithMiddleware.
func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("GET /api/drives", s.handleDrives)
	mux.HandleFunc("GET /api/browse", s.handleBrowse)
	mux.HandleFunc("GET /api/models", s.handleModels)
	mux.HandleFunc("GET /api/languages", s.handleLanguages)
	mux.HandleFunc("GET /api/settings", s.handleGetSettings)
	mux.HandleFunc("PUT /api/settings", s.handleUpdateSettings)
	mux.HandleFunc("POST /api/jobs", s.handleCreateJob)
	mux.HandleFunc("GET /api/jobs", s.handleListJobs)
	mux.HandleFunc("GET /api/jobs/{id}", s.handleGetJob)
	mux.HandleFunc("GET /api/jobs/{id}/events", s.handleJobEvents)
	mux.HandleFunc("POST /api/jobs/{id}/cancel", s.handleCancelJob)
	mux.HandleFunc("POST /api/jobs/{id}/export", s.handleExportJob)
	// Anything else under /api/ is a JSON 404 rather than falling through to
	// the SPA's index.html.
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeErrorMsg(w, http.StatusNotFound, "no such API endpoint: "+r.Method+" "+r.URL.Path)
	})
}
