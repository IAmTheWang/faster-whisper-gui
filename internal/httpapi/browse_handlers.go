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

// handleBrowse lists a directory's contents. The optional "kind" query
// param controls which files (besides subdirectories, always included)
// show up in the listing:
//
//   - "" / "media" (default): video/audio files — the original media-picker behavior.
//   - "exe": .exe files — used by the Settings panel to browse for
//     ffmpeg.exe/whisper-cli.exe.
//   - "dir": no files at all — used by the Settings panel to browse for the
//     models directory, where there's nothing to click within a directory.
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

	var listing fsbrowse.Listing
	switch r.URL.Query().Get("kind") {
	case "exe":
		listing, err = fsbrowse.ListFiltered(dir, func(name string) string {
			if fsbrowse.IsExeFile(name) {
				return "file"
			}
			return ""
		})
	case "dir":
		listing, err = fsbrowse.ListFiltered(dir, func(string) string { return "" })
	default:
		listing, err = fsbrowse.List(dir)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, listing)
}
