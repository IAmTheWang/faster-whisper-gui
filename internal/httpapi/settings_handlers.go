package httpapi

import (
	"encoding/json"
	"net/http"

	"faster-whisper-gui/internal/fsbrowse"
)

// settingsResponse reports both the user's current overrides and the
// conventional defaults they fall back to, so the frontend can show
// "currently using default: ..." placeholder text when a field has no
// override set.
type settingsResponse struct {
	FfmpegPath      string `json:"ffmpegPath"`
	WhisperCliPath  string `json:"whisperCliPath"`
	ModelsDir       string `json:"modelsDir"`
	DefaultVideoDir string `json:"defaultVideoDir"`

	FfmpegDefault     string `json:"ffmpegDefault"`
	WhisperCliDefault string `json:"whisperCliDefault"`
	ModelsDirDefault  string `json:"modelsDirDefault"`
}

func (s *Server) settingsResponse() settingsResponse {
	p := s.Config.Settings.Get()
	return settingsResponse{
		FfmpegPath:      p.FfmpegPath,
		WhisperCliPath:  p.WhisperCliPath,
		ModelsDir:       p.ModelsDir,
		DefaultVideoDir: p.DefaultVideoDir,

		FfmpegDefault:     s.Config.FfmpegPath,
		WhisperCliDefault: s.Config.WhisperCliPath,
		ModelsDirDefault:  s.Config.ModelsDir,
	}
}

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.settingsResponse())
}

// updateSettingsRequest uses *string, not string, for each field so the
// handler can distinguish three states a plain string+omitempty can't:
// the field is absent from the JSON body (nil, leave that setting
// untouched), present as "" (explicitly clear the override back to
// default), or present as a real path (validate and store it).
type updateSettingsRequest struct {
	FfmpegPath      *string `json:"ffmpegPath"`
	WhisperCliPath  *string `json:"whisperCliPath"`
	ModelsDir       *string `json:"modelsDir"`
	DefaultVideoDir *string `json:"defaultVideoDir"`
}

// handleUpdateSettings validates and persists any fields present in the
// request body, leaving fields not present untouched.
func (s *Server) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	var req updateSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorMsg(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}

	current := s.Config.Settings.Get()

	if req.FfmpegPath != nil {
		if *req.FfmpegPath == "" {
			current.FfmpegPath = ""
		} else {
			p, err := fsbrowse.ValidateExeFile(*req.FfmpegPath)
			if err != nil {
				writeErrorMsg(w, http.StatusBadRequest, "invalid ffmpeg path: "+err.Error())
				return
			}
			current.FfmpegPath = p
		}
	}

	if req.WhisperCliPath != nil {
		if *req.WhisperCliPath == "" {
			current.WhisperCliPath = ""
		} else {
			p, err := fsbrowse.ValidateExeFile(*req.WhisperCliPath)
			if err != nil {
				writeErrorMsg(w, http.StatusBadRequest, "invalid whisper-cli path: "+err.Error())
				return
			}
			current.WhisperCliPath = p
		}
	}

	if req.ModelsDir != nil {
		if *req.ModelsDir == "" {
			current.ModelsDir = ""
		} else {
			d, err := fsbrowse.ValidateAbsDir(*req.ModelsDir)
			if err != nil {
				writeErrorMsg(w, http.StatusBadRequest, "invalid models directory: "+err.Error())
				return
			}
			current.ModelsDir = d
		}
	}

	if req.DefaultVideoDir != nil {
		if *req.DefaultVideoDir == "" {
			current.DefaultVideoDir = ""
		} else {
			d, err := fsbrowse.ValidateAbsDir(*req.DefaultVideoDir)
			if err != nil {
				writeErrorMsg(w, http.StatusBadRequest, "invalid default video directory: "+err.Error())
				return
			}
			current.DefaultVideoDir = d
		}
	}

	if err := s.Config.Settings.Set(current); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, s.settingsResponse())
}
