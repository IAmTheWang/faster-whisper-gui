// Package httpapi wires the REST/SSE API described in the project plan on
// top of internal/job, internal/fsbrowse, and internal/transcribe. It only
// registers /api/* routes — cmd/server/main.go composes those with the
// embedded frontend and owns the actual http.Server.
package httpapi

import (
	"faster-whisper-gui/internal/config"
	"faster-whisper-gui/internal/job"
	"faster-whisper-gui/internal/sseutil"
)

// Server holds every dependency the API handlers need.
type Server struct {
	Config *config.Config
	Store  *job.Store
	Queue  *job.Queue
	Broker *sseutil.Broker
}

func NewServer(cfg *config.Config, store *job.Store, queue *job.Queue, broker *sseutil.Broker) *Server {
	return &Server{Config: cfg, Store: store, Queue: queue, Broker: broker}
}
