// Package settings persists user-chosen overrides for the ffmpeg/whisper-cli
// binary paths and the models directory, so the web UI can point at files
// that live outside the conventional bin/whisper, bin/ffmpeg, models
// layout (see internal/config) without the user hand-editing anything on
// disk.
package settings

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"

	"faster-whisper-gui/internal/atomicfile"
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

	writeMu sync.Mutex // serializes Set/Update, including the disk write

	mu      sync.RWMutex // guards current
	current Paths
}

// Load reads filePath into a Store. A missing file is not an error — it
// means no overrides have been saved yet, so Get returns a zero-value
// Paths. A file that exists but fails to parse is moved aside to
// "<filePath>.bad" (logged) and treated as empty, so a corrupt file can't
// keep the app from starting while still being kept for inspection; Load only
// errors if it can't even move the file aside.
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
		// A truncated/garbled settings file (e.g. a crash mid-write on an older
		// version) must not stop the app from starting. Keep the bad file for
		// inspection and fall back to the defaults.
		s.current = Paths{}
		bad := filePath + ".bad"
		os.Remove(bad)
		if rerr := os.Rename(filePath, bad); rerr != nil {
			return nil, fmt.Errorf("parse settings file %s: %w (and could not move it aside: %v)", filePath, err, rerr)
		}
		log.Printf("settings: %s is corrupt (%v); moved to %s and using defaults", filePath, err, bad)
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
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.setLocked(p)
}

// Update applies fn to the current value and persists the result as one
// atomic read-modify-write, so two concurrent updates to different fields
// can't overwrite each other. If fn returns an error nothing is written.
func (s *Store) Update(fn func(*Paths) error) (Paths, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	p := s.Get()
	if err := fn(&p); err != nil {
		return p, err
	}
	if err := s.setLocked(p); err != nil {
		return p, err
	}
	return p, nil
}

// setLocked does the write; callers must hold writeMu.
func (s *Store) setLocked(p Paths) error {
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}

	if err := atomicfile.WriteBytes(s.filePath, data); err != nil {
		return fmt.Errorf("write settings file: %w", err)
	}

	s.mu.Lock()
	s.current = p
	s.mu.Unlock()
	return nil
}
