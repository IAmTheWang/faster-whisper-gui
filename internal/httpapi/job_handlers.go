package httpapi

import (
	"encoding/json"
	"net/http"

	"faster-whisper-gui/internal/fsbrowse"
	"faster-whisper-gui/internal/job"
	"faster-whisper-gui/internal/transcribe"
)

type createJobRequest struct {
	VideoPath  string `json:"videoPath"`
	ModelID    string `json:"modelId"`
	Language   string `json:"language"`
	OutputMode string `json:"outputMode"`
	OutputDir  string `json:"outputDir,omitempty"`
	MaxLen     int    `json:"maxLen,omitempty"`
}

type createJobResponse struct {
	JobID  string `json:"jobId"`
	Status string `json:"status"`
}

// handleCreateJob validates the request end to end — video exists, output
// directory exists, model is known, target SRT path is currently writable —
// before enqueueing, so a bad request fails immediately instead of after a
// possibly multi-minute transcription run.
func (s *Server) handleCreateJob(w http.ResponseWriter, r *http.Request) {
	var req createJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorMsg(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}

	videoPath, err := fsbrowse.ValidateVideoFile(req.VideoPath)
	if err != nil {
		writeErrorMsg(w, http.StatusBadRequest, "视频路径无效: "+err.Error())
		return
	}

	outputMode := job.OutputMode(req.OutputMode)
	if outputMode == "" {
		outputMode = job.OutputSameAsVideo
	}

	var outputDir string
	switch outputMode {
	case job.OutputSameAsVideo:
		// no extra validation needed — the video's own directory is already known-good
	case job.OutputCustom:
		outputDir, err = fsbrowse.ValidateAbsDir(req.OutputDir)
		if err != nil {
			writeErrorMsg(w, http.StatusBadRequest, "输出目录无效: "+err.Error())
			return
		}
	default:
		writeErrorMsg(w, http.StatusBadRequest, "未知的 outputMode: "+req.OutputMode)
		return
	}

	models, err := transcribe.ScanModels(s.Config.ModelsDir)
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
		writeErrorMsg(w, http.StatusBadRequest, "未找到模型: "+req.ModelID)
		return
	}

	jobReq := job.Request{
		VideoPath:  videoPath,
		ModelID:    req.ModelID,
		ModelPath:  modelPath,
		Language:   req.Language,
		OutputMode: outputMode,
		OutputDir:  outputDir,
		MaxLen:     req.MaxLen,
	}

	srtPath := job.OutputPrefix(jobReq) + ".srt"
	if err := fsbrowse.EnsureWritable(srtPath); err != nil {
		writeErrorMsg(w, http.StatusConflict, "目标字幕文件不可写（可能正被播放器占用）: "+err.Error())
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
