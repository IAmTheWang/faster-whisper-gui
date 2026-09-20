package httpapi

import (
	"encoding/json"
	"net/http"
	"path/filepath"

	"faster-whisper-gui/internal/fsbrowse"
	"faster-whisper-gui/internal/job"
	"faster-whisper-gui/internal/transcribe"
)

type createJobRequest struct {
	MediaPath  string `json:"mediaPath"`
	ModelID    string `json:"modelId"`
	Language   string `json:"language"`
	OutputMode string `json:"outputMode"`
	OutputDir  string `json:"outputDir,omitempty"`
	MaxLen     int    `json:"maxLen,omitempty"`
	MaxContext int    `json:"maxContext,omitempty"`
}

type createJobResponse struct {
	JobID  string `json:"jobId"`
	Status string `json:"status"`
}

// handleCreateJob validates the request end to end — media file exists,
// output directory exists, model is known, target SRT path is currently
// writable — before enqueueing, so a bad request fails immediately instead
// of after a possibly multi-minute transcription run.
func (s *Server) handleCreateJob(w http.ResponseWriter, r *http.Request) {
	var req createJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorMsg(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}

	mediaPath, err := fsbrowse.ValidateMediaFile(req.MediaPath)
	if err != nil {
		writeErrorMsg(w, http.StatusBadRequest, "invalid media path: "+err.Error())
		return
	}

	outputMode := job.OutputMode(req.OutputMode)
	if outputMode == "" {
		outputMode = job.OutputSameAsSource
	}

	var outputDir string
	switch outputMode {
	case job.OutputSameAsSource:
		// no extra validation needed — the source file's own directory is already known-good
	case job.OutputCustom:
		outputDir, err = fsbrowse.ValidateAbsDir(req.OutputDir)
		if err != nil {
			writeErrorMsg(w, http.StatusBadRequest, "invalid output directory: "+err.Error())
			return
		}
	default:
		writeErrorMsg(w, http.StatusBadRequest, "unknown outputMode: "+req.OutputMode)
		return
	}

	models, err := transcribe.ScanModels(s.Config.EffectiveModelsDir())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	var modelPath string
	for _, m := range models {
		if m.ID == req.ModelID {
			modelPath = m.Path
			break
		}
	}
	if modelPath == "" {
		writeErrorMsg(w, http.StatusBadRequest, "model not found: "+req.ModelID)
		return
	}

	jobReq := job.Request{
		MediaPath:  mediaPath,
		ModelID:    req.ModelID,
		ModelPath:  modelPath,
		Language:   req.Language,
		OutputMode: outputMode,
		OutputDir:  outputDir,
		MaxLen:     req.MaxLen,
		MaxContext: req.MaxContext,
	}

	srtPath := job.OutputPrefix(jobReq) + ".srt"
	if err := fsbrowse.EnsureWritable(srtPath); err != nil {
		writeErrorMsg(w, http.StatusConflict, "target subtitle file is not writable (it may be open in a media player): "+err.Error())
		return
	}

	j, err := s.Queue.Submit(jobReq)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusAccepted, createJobResponse{JobID: j.ID, Status: string(j.Status)})
}

func (s *Server) handleListJobs(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.Store.List())
}

func (s *Server) handleGetJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	j, ok := s.Store.Get(id)
	if !ok {
		writeErrorMsg(w, http.StatusNotFound, "job not found")
		return
	}
	writeJSON(w, http.StatusOK, j)
}

func (s *Server) handleCancelJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := s.Store.Get(id); !ok {
		writeErrorMsg(w, http.StatusNotFound, "job not found")
		return
	}
	if !s.Store.Cancel(id) {
		writeErrorMsg(w, http.StatusConflict, "job is not currently running")
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

type exportJobRequest struct {
	DestDir string `json:"destDir"`
}

type exportJobResponse struct {
	DestPath string `json:"destPath"`
}

// handleExportJob copies an already-finished job's SRT into a user-chosen
// folder, on top of the automatic write it already got next to the source
// file (or its custom output dir) when the job completed. This is an additive
// "save a copy" action, not a replacement for that automatic write.
func (s *Server) handleExportJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	j, ok := s.Store.Get(id)
	if !ok {
		writeErrorMsg(w, http.StatusNotFound, "job not found")
		return
	}
	if j.Status != job.StatusDone {
		writeErrorMsg(w, http.StatusConflict, "job has not finished yet")
		return
	}

	var req exportJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorMsg(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	destDir, err := fsbrowse.ValidateAbsDir(req.DestDir)
	if err != nil {
		writeErrorMsg(w, http.StatusBadRequest, "invalid destination directory: "+err.Error())
		return
	}

	destPath := filepath.Join(destDir, filepath.Base(j.SRTPath))
	if err := fsbrowse.CopyFile(j.SRTPath, destPath); err != nil {
		writeErrorMsg(w, http.StatusConflict, "could not copy subtitle file: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, exportJobResponse{DestPath: destPath})
}
