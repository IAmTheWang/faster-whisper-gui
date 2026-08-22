package httpapi

import (
	"net/http"

	"faster-whisper-gui/internal/fsbrowse"
)

func (s *Server) handleDrives(w http.ResponseWriter, r *http.Request) {
	drives, err := fsbrowse.ListDrives()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, drives)
}

func (s *Server) handleBrowse(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("path")
	if raw == "" {
		writeErrorMsg(w, http.StatusBadRequest, "missing path query parameter")
		return
	}

	dir, err := fsbrowse.ValidateAbsDir(raw)
	if err != nil {
		writeErrorMsg(w, http.StatusBadRequest, err.Error())
		return
	}

	listing, err := fsbrowse.List(dir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, listing)
}
