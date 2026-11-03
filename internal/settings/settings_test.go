package settings

import (
	"os"
	"path/filepath"
	"sync"
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

func TestUpdateConcurrentFieldsDoNotClobber(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	setters := []func(*Paths){
		func(p *Paths) { p.FfmpegPath = `C:\a\ffmpeg.exe` },
		func(p *Paths) { p.WhisperCliPath = `C:\a\whisper-cli.exe` },
		func(p *Paths) { p.ModelsDir = `C:\a\models` },
		func(p *Paths) { p.DefaultVideoDir = `C:\a\videos` },
	}
	var wg sync.WaitGroup
	for _, set := range setters {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 10; i++ {
				if _, err := s.Update(func(p *Paths) error { set(p); return nil }); err != nil {
					t.Error(err)
				}
			}
		}()
	}
	wg.Wait()

	reloaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	got := reloaded.Get()
	if got.FfmpegPath == "" || got.WhisperCliPath == "" || got.ModelsDir == "" || got.DefaultVideoDir == "" {
		t.Fatalf("an update was lost: %+v", got)
	}
}

func TestUpdateErrorWritesNothing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s, _ := Load(path)
	_, err := s.Update(func(p *Paths) error { p.ModelsDir = "x"; return os.ErrInvalid })
	if err == nil {
		t.Fatal("want error")
	}
	if s.Get().ModelsDir != "" {
		t.Fatal("failed Update must not change the stored value")
	}
	if _, statErr := os.Stat(path); statErr == nil {
		t.Fatal("failed Update must not create settings.json")
	}
}
