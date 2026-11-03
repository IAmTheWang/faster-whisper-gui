package httpapi

import (
	"errors"
	"net/http"

	"faster-whisper-gui/internal/fsbrowse"
	"faster-whisper-gui/internal/settings"
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

// pathError marks a validation failure (the client's fault, 400) as opposed
// to a failure persisting settings.json (500).
type pathError struct{ msg string }

func (e *pathError) Error() string { return e.msg }

// applyPath implements the tri-state PUT semantics for one field: nil
// leaves *dst untouched, "" clears the override, anything else must pass
// validate and is stored in its cleaned form.
func applyPath(dst *string, req *string, validate func(string) (string, error), what string) error {
	if req == nil {
		return nil
	}
	if *req == "" {
		*dst = ""
		return nil
	}
	p, err := validate(*req)
	if err != nil {
		return &pathError{"invalid " + what + ": " + err.Error()}
	}
	*dst = p
	return nil
}

// handleUpdateSettings validates and persists any fields present in the
// request body, leaving fields not present untouched.
func (s *Server) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	var req updateSettingsRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeErrorMsg(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}

	_, err := s.Config.Settings.Update(func(cur *settings.Paths) error {
		return errors.Join(
			applyPath(&cur.FfmpegPath, req.FfmpegPath, fsbrowse.ValidateExeFile, "ffmpeg path"),
			applyPath(&cur.WhisperCliPath, req.WhisperCliPath, fsbrowse.ValidateExeFile, "whisper-cli path"),
			applyPath(&cur.ModelsDir, req.ModelsDir, fsbrowse.ValidateAbsDir, "models directory"),
			applyPath(&cur.DefaultVideoDir, req.DefaultVideoDir, fsbrowse.ValidateAbsDir, "default media directory"),
		)
	})
	if err != nil {
		var ve *pathError
		if errors.As(err, &ve) {
			writeError(w, http.StatusBadRequest, err)
		} else {
			writeError(w, http.StatusInternalServerError, err)
		}
		return
	}

	writeJSON(w, http.StatusOK, s.settingsResponse())
}
