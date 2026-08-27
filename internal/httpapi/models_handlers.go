package httpapi

import (
	"net/http"

	"faster-whisper-gui/internal/transcribe"
)

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	models, err := transcribe.ScanModels(s.Config.EffectiveModelsDir())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if models == nil {
		models = []transcribe.Model{}
	}
	writeJSON(w, http.StatusOK, models)
}

// Language is one selectable entry for the job config panel's language
// dropdown.
type Language struct {
	Code  string `json:"code"`
	Label string `json:"label"`
}

// languages is a small curated list, not an exhaustive one — whisper.cpp
// supports far more, but the UI only needs common ones plus auto-detect.
var languages = []Language{
	{"auto", "Auto-detect"},
	{"en", "English"},
	{"zh", "中文"},
	{"ja", "日本語"},
	{"ko", "한국어"},
	{"es", "Español"},
	{"fr", "Français"},
	{"de", "Deutsch"},
	{"ru", "Русский"},
	{"pt", "Português"},
	{"it", "Italiano"},
}

func (s *Server) handleLanguages(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, languages)
}
