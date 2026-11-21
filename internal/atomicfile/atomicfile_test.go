package atomicfile

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func entries(t *testing.T, dir string) []string {
	t.Helper()
	list, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range list {
		names = append(names, e.Name())
	}
	return names
}

func TestWriteBytesCreatesAndReplaces(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.txt")

	if err := WriteBytes(path, []byte("first")); err != nil {
		t.Fatal(err)
	}
	if err := WriteBytes(path, []byte("second")); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != "second" {
		t.Fatalf("content = %q, want second", got)
	}
	if names := entries(t, dir); len(names) != 1 {
		t.Fatalf("stray files left behind: %v", names)
	}
}

func TestFailedFillLeavesTargetUntouchedAndNoTempFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.txt")
	if err := os.WriteFile(path, []byte("keep me"), 0o644); err != nil {
		t.Fatal(err)
	}

	boom := errors.New("boom")
	err := Write(path, func(w io.Writer) error {
		w.Write([]byte("partial"))
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
	if got, _ := os.ReadFile(path); string(got) != "keep me" {
		t.Fatalf("target changed to %q by a failed write", got)
	}
	if names := entries(t, dir); len(names) != 1 {
		t.Fatalf("temp file not cleaned up: %v", names)
	}
}

func TestWriteIntoMissingDirectoryFails(t *testing.T) {
	if err := WriteBytes(filepath.Join(t.TempDir(), "no", "such", "dir", "f"), []byte("x")); err == nil {
		t.Fatal("want an error when the directory does not exist")
	}
}
