package settings

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingFile(t *testing.T) {
	dir := t.TempDir()
	s, err := Load(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := s.Get(); got != (Paths{}) {
		t.Fatalf("Get() = %+v, want zero value", got)
	}
}

func TestLoadCorruptedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(path, []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("Load with corrupted file: want error, got nil")
	}
}

func TestSetAndLoadRoundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	want := Paths{
		FfmpegPath:      `C:\tools\ffmpeg.exe`,
		WhisperCliPath:  `C:\tools\whisper-cli.exe`,
		ModelsDir:       `D:\models`,
		DefaultVideoDir: `H:\videos`,
	}
	if err := s.Set(want); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if got := s.Get(); got != want {
		t.Fatalf("Get() after Set = %+v, want %+v", got, want)
	}

	reloaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load after Set: %v", err)
	}
	if got := reloaded.Get(); got != want {
		t.Fatalf("reloaded Get() = %+v, want %+v", got, want)
	}
}

// TestSetLeavesNoTempFiles guards the atomic-write behavior: after a
// successful Set, only settings.json should exist in the directory — no
// leftover *.tmp files from the temp-file+rename dance.
func TestSetLeavesNoTempFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Set(Paths{FfmpegPath: `C:\ffmpeg.exe`}); err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "settings.json" {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("directory entries after Set = %v, want only settings.json", names)
	}
}
