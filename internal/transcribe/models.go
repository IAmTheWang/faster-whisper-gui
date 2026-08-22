package transcribe

import (
	"os"
	"path/filepath"
	"strings"
)

// Model describes a ggml model file available for transcription.
type Model struct {
	ID        string `json:"id"`
	Filename  string `json:"filename"`
	Path      string `json:"path"`
	SizeBytes int64  `json:"sizeBytes"`
	Label     string `json:"label"`
}

// ScanModels lists ggml-*.bin files directly inside modelsDir (no
// recursion — MVP scope is "user manually drops model files in here"). The
// ID is derived from the filename, e.g. "ggml-small.bin" -> "small".
func ScanModels(modelsDir string) ([]Model, error) {
	entries, err := os.ReadDir(modelsDir)
	if err != nil {
		return nil, err
	}

	var models []Model
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		lower := strings.ToLower(name)
		if !strings.HasPrefix(lower, "ggml-") || !strings.HasSuffix(lower, ".bin") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		id := strings.TrimSuffix(strings.TrimPrefix(lower, "ggml-"), ".bin")
		models = append(models, Model{
			ID:        id,
			Filename:  name,
			Path:      filepath.Join(modelsDir, name),
			SizeBytes: info.Size(),
			Label:     id,
		})
	}
	return models, nil
}
