// Package httpapi wires the REST/SSE API described in the project plan on
// top of internal/job, internal/fsbrowse, and internal/transcribe. It only
// registers /api/* routes — cmd/server/main.go composes those with the
// embedded frontend and owns the actual http.Server.
package httpapi

import (
	"sync"

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

	health healthCache

	// streamsDone is closed by CloseStreams so long-lived SSE handlers return
	// during a graceful shutdown (they would otherwise hold it open until its
	// deadline). nil — as in tests that build a Server literal — never fires.
	streamsDone chan struct{}
	closeOnce   sync.Once
}

func NewServer(cfg *config.Config, store *job.Store, queue *job.Queue, broker *sseutil.Broker) *Server {
	return &Server{Config: cfg, Store: store, Queue: queue, Broker: broker, streamsDone: make(chan struct{})}
}

// CloseStreams ends every open all-jobs event stream. Safe to call repeatedly;
// wire it to http.Server.RegisterOnShutdown.
func (s *Server) CloseStreams() {
	if s.streamsDone == nil {
		return
	}
	s.closeOnce.Do(func() { close(s.streamsDone) })
}
