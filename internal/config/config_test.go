package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEffectivePathsFallBackToDefaults(t *testing.T) {
	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if got := cfg.EffectiveFfmpegPath(); got != cfg.FfmpegPath {
		t.Errorf("EffectiveFfmpegPath() = %q, want default %q", got, cfg.FfmpegPath)
	}
	if got := cfg.EffectiveFfprobePath(); got != cfg.FfprobePath {
		t.Errorf("EffectiveFfprobePath() = %q, want default %q", got, cfg.FfprobePath)
	}
	if got := cfg.EffectiveWhisperCliPath(); got != cfg.WhisperCliPath {
		t.Errorf("EffectiveWhisperCliPath() = %q, want default %q", got, cfg.WhisperCliPath)
	}
	if got := cfg.EffectiveModelsDir(); got != cfg.ModelsDir {
		t.Errorf("EffectiveModelsDir() = %q, want default %q", got, cfg.ModelsDir)
	}
	if got := cfg.EffectiveDefaultVideoDir(); got != "" {
		t.Errorf("EffectiveDefaultVideoDir() = %q, want empty (no conventional default)", got)
	}
}

func TestEffectivePathsUseOverrides(t *testing.T) {
	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	overrideFfmpeg := `D:\tools\ffmpeg\ffmpeg.exe`
	overrideWhisper := `D:\tools\whisper\whisper-cli.exe`
	overrideModels := `D:\models`
	overrideVideoDir := `H:\videos`

	if err := cfg.Settings.Set(cfg.Settings.Get()); err != nil {
		t.Fatal(err)
	}
	overridden := cfg.Settings.Get()
	overridden.FfmpegPath = overrideFfmpeg
	overridden.WhisperCliPath = overrideWhisper
	overridden.ModelsDir = overrideModels
	overridden.DefaultVideoDir = overrideVideoDir
	if err := cfg.Settings.Set(overridden); err != nil {
		t.Fatal(err)
	}

	if got := cfg.EffectiveFfmpegPath(); got != overrideFfmpeg {
		t.Errorf("EffectiveFfmpegPath() = %q, want override %q", got, overrideFfmpeg)
	}
	wantFfprobe := filepath.Join(filepath.Dir(overrideFfmpeg), "ffprobe.exe")
	if got := cfg.EffectiveFfprobePath(); got != wantFfprobe {
		t.Errorf("EffectiveFfprobePath() = %q, want %q (derived from overridden ffmpeg dir)", got, wantFfprobe)
	}
	if got := cfg.EffectiveWhisperCliPath(); got != overrideWhisper {
		t.Errorf("EffectiveWhisperCliPath() = %q, want override %q", got, overrideWhisper)
	}
	if got := cfg.EffectiveModelsDir(); got != overrideModels {
		t.Errorf("EffectiveModelsDir() = %q, want override %q", got, overrideModels)
	}
	if got := cfg.EffectiveDefaultVideoDir(); got != overrideVideoDir {
		t.Errorf("EffectiveDefaultVideoDir() = %q, want override %q", got, overrideVideoDir)
	}
}

func TestIsGoRunDir(t *testing.T) {
	tmp := os.TempDir()
	if !isGoRunDir(filepath.Join(tmp, "go-build123456", "b001", "exe")) {
		t.Error("go run build dir under TEMP should be detected")
	}
	if isGoRunDir(filepath.Join(tmp, "my-tool")) {
		t.Error("ordinary temp subdirectory is not a go run dir")
	}
	if isGoRunDir(`E:\tools\go-build-helper`) {
		t.Error("a real install path containing go-build must not be treated as go run")
	}
}

func TestLoadCreatesLayoutAndCleanTmpDirWipesIt(t *testing.T) {
	root := t.TempDir()
	cfg, err := Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, d := range []string{cfg.WhisperDir, cfg.FfmpegDir, cfg.ModelsDir, cfg.TmpDir} {
		if info, err := os.Stat(d); err != nil || !info.IsDir() {
			t.Errorf("expected %s to be created by Load: %v", d, err)
		}
	}
	if cfg.Settings == nil {
		t.Fatal("Load must attach a settings store")
	}

	stale := filepath.Join(cfg.TmpDir, "stale.wav")
	if err := os.WriteFile(stale, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(cfg.TmpDir, "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := cfg.CleanTmpDir(); err != nil {
		t.Fatalf("CleanTmpDir: %v", err)
	}
	entries, _ := os.ReadDir(cfg.TmpDir)
	if len(entries) != 0 {
		t.Fatalf("tmp dir not emptied: %v", entries)
	}
	if _, err := os.Stat(cfg.TmpDir); err != nil {
		t.Fatal("CleanTmpDir must keep the directory itself")
	}
}

func TestLoadRecoversFromCorruptSettings(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "settings.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(root)
	if err != nil {
		t.Fatalf("a corrupt settings.json must not block startup: %v", err)
	}
	if got := cfg.EffectiveModelsDir(); got != cfg.ModelsDir {
		t.Fatalf("EffectiveModelsDir = %q, want default %q", got, cfg.ModelsDir)
	}
	if _, err := os.Stat(filepath.Join(root, "settings.json.bad")); err != nil {
		t.Fatalf("corrupt file should be kept as settings.json.bad: %v", err)
	}
}
