package transcribe

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanModels(t *testing.T) {
	dir := t.TempDir()
	for name, content := range map[string]string{
		"ggml-small.bin":     "12345",
		"GGML-Large-V3.BIN":  "1",
		"readme.txt":         "x",
		"ggml-notamodel.txt": "x",
		"small.bin":          "x",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "ggml-dir.bin"), 0o755); err != nil {
		t.Fatal(err)
	}

	models, err := ScanModels(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]Model{}
	for _, m := range models {
		got[m.ID] = m
	}
	if len(got) != 2 || got["small"].SizeBytes != 5 || got["large-v3"].Filename != "GGML-Large-V3.BIN" {
		t.Fatalf("unexpected models: %+v", models)
	}
	if got["small"].Path != filepath.Join(dir, "ggml-small.bin") {
		t.Errorf("Path = %q", got["small"].Path)
	}
}

func TestScanModelsMissingDir(t *testing.T) {
	if _, err := ScanModels(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Fatal("want an error for a missing models directory")
	}
}
