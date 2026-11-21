// Package atomicfile writes a file so a crash or failure mid-write can never
// leave a partial or zero-length file at the destination: the content goes to
// a temp file in the same directory, is fsynced, and is then renamed over the
// target (atomic on the same volume). It has no dependencies on the rest of
// the repo.
package atomicfile

import (
	"io"
	"os"
	"path/filepath"
)

// Write creates a temp file beside path, lets fill write its content, syncs it
// to disk and renames it over path. On any failure the temp file is removed and
// path is left untouched.
func Write(path string, fill func(io.Writer) error) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".atomic-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	fail := func(err error) error {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := fill(tmp); err != nil {
		return fail(err)
	}
	// Without the sync a power loss after the rename can leave a zero-length
	// file: the rename is journaled, the data blocks may not be.
	if err := tmp.Sync(); err != nil {
		return fail(err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return nil
}

// WriteBytes is Write for an in-memory payload.
func WriteBytes(path string, data []byte) error {
	return Write(path, func(w io.Writer) error {
		_, err := w.Write(data)
		return err
	})
}
