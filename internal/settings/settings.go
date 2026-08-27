// Package settings persists user-chosen overrides for the ffmpeg/whisper-cli
// binary paths and the models directory, so the web UI can point at files
// that live outside the conventional bin/whisper, bin/ffmpeg, models
// layout (see internal/config) without the user hand-editing anything on
// disk.
package settings

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Paths holds the user's overrides. An empty field means "use the
// conventional default from internal/config" — there is no separate
// tri-state here; that distinction only matters at the HTTP request layer
// (see internal/httpapi's settings handlers), not in what ends up on disk.
type Paths struct {
	FfmpegPath      string `json:"ffmpegPath,omitempty"`
	WhisperCliPath  string `json:"whisperCliPath,omitempty"`
	ModelsDir       string `json:"modelsDir,omitempty"`
	DefaultVideoDir string `json:"defaultVideoDir,omitempty"`
}

// Store is a concurrency-safe holder for Paths, backed by a JSON file on
// disk.
type Store struct {
	filePath string

	mu      sync.RWMutex
	current Paths
}

// Load reads filePath into a Store. A missing file is not an error — it
// means no overrides have been saved yet, so Get returns a zero-value
// Paths. A file that exists but fails to parse *is* an error, so a
// corrupted settings file surfaces at startup instead of silently
// discarding whatever the user last saved.
func Load(filePath string) (*Store, error) {
	s := &Store{filePath: filePath}

	data, err := os.ReadFile(filePath)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read settings file %s: %w", filePath, err)
	}

	if err := json.Unmarshal(data, &s.current); err != nil {
		return nil, fmt.Errorf("parse settings file %s: %w", filePath, err)
	}
	return s, nil
}

// Get returns the current overrides.
func (s *Store) Get() Paths {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.current
}

// Set persists p to disk and, only once that succeeds, updates the
// in-memory value returned by subsequent Get calls. The write is atomic
// (temp file + rename) so a crash mid-write can't leave a half-written
// settings.json behind for the next Load to choke on.
func (s *Store) Set(p Paths) error {
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(s.filePath), ".settings-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp settings file: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write temp settings file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("close temp settings file: %w", err)
	}

	if err := os.Rename(tmpPath, s.filePath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("replace settings file: %w", err)
	}

	s.mu.Lock()
	s.current = p
	s.mu.Unlock()
	return nil
}
