package httpapi

import (
	"net/http"

	"faster-whisper-gui/internal/transcribe"
)

type healthResponse struct {
	Ffmpeg     transcribe.ComponentHealth `json:"ffmpeg"`
	WhisperCli transcribe.ComponentHealth `json:"whisperCli"`
}

// handleHealth probes ffmpeg/whisper-cli so missing binaries or DLLs surface
// as a clear message at page load instead of after a user submits a job.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	writeJSON(w, http.StatusOK, healthResponse{
		Ffmpeg:     transcribe.CheckFfmpeg(ctx, s.Config.EffectiveFfmpegPath()),
		WhisperCli: transcribe.CheckWhisperCli(ctx, s.Config.EffectiveWhisperCliPath()),
	})
}
