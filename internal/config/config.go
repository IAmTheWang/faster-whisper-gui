// Package config holds the server's runtime configuration: where the
// bundled binaries/models live and which port to listen on.
package config

import (
	"os"
	"path/filepath"

	"faster-whisper-gui/internal/settings"
)

// Config holds resolved absolute paths to everything the server needs on
// disk. All paths are rooted at the directory containing the running
// executable (or the current working directory in `go run`/tests), not the
// user's current directory, so the tool behaves the same regardless of where
// it's launched from.
type Config struct {
	Addr string // e.g. "127.0.0.1:8080"

	Root       string // repo/executable root
	WhisperDir string // bin/whisper
	FfmpegDir  string // bin/ffmpeg
	ModelsDir  string // models
	TmpDir     string // tmp

	WhisperCliPath string // bin/whisper/whisper-cli.exe
	FfmpegPath     string // bin/ffmpeg/ffmpeg.exe
	FfprobePath    string // bin/ffmpeg/ffprobe.exe

	// Settings holds user overrides for the paths above, saved from the web
	// UI. Use the Effective* methods, not the fields above directly, when
	// resolving what path to actually use.
	Settings *settings.Store
}

// Load resolves a Config rooted at root (pass "" to use the directory
// containing the current executable). It creates the conventional
// directories (bin/whisper, bin/ffmpeg, models, tmp) if they don't exist yet
// — a fresh checkout won't have them, and downstream directory-scanning code
// should never have to special-case ErrNotExist.
func Load(root string) (*Config, error) {
	if root == "" {
		exe, err := os.Executable()
		if err != nil {
			return nil, err
		}
		root = filepath.Dir(exe)
	}

	cfg := &Config{
		Addr:       "127.0.0.1:8080",
		Root:       root,
		WhisperDir: filepath.Join(root, "bin", "whisper"),
		FfmpegDir:  filepath.Join(root, "bin", "ffmpeg"),
		ModelsDir:  filepath.Join(root, "models"),
		TmpDir:     filepath.Join(root, "tmp"),
	}
	cfg.WhisperCliPath = filepath.Join(cfg.WhisperDir, "whisper-cli.exe")
	cfg.FfmpegPath = filepath.Join(cfg.FfmpegDir, "ffmpeg.exe")
	cfg.FfprobePath = filepath.Join(cfg.FfmpegDir, "ffprobe.exe")

	for _, dir := range []string{cfg.WhisperDir, cfg.FfmpegDir, cfg.ModelsDir, cfg.TmpDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}

	settingsStore, err := settings.Load(filepath.Join(root, "settings.json"))
	if err != nil {
		return nil, err
	}
	cfg.Settings = settingsStore

	return cfg, nil
}

// EffectiveFfmpegPath returns the user-configured ffmpeg.exe override if
// one is set, else the conventional bin/ffmpeg/ffmpeg.exe default.
func (c *Config) EffectiveFfmpegPath() string {
	if p := c.Settings.Get().FfmpegPath; p != "" {
		return p
	}
	return c.FfmpegPath
}

// EffectiveFfprobePath returns ffprobe.exe next to the overridden
// ffmpeg.exe (ffmpeg distributions always ship both binaries side by side),
// or the conventional bin/ffmpeg/ffprobe.exe default if ffmpeg isn't
// overridden.
func (c *Config) EffectiveFfprobePath() string {
	if p := c.Settings.Get().FfmpegPath; p != "" {
		return filepath.Join(filepath.Dir(p), "ffprobe.exe")
	}
	return c.FfprobePath
}

// EffectiveWhisperCliPath returns the user-configured whisper-cli.exe
// override if one is set, else the conventional
// bin/whisper/whisper-cli.exe default.
func (c *Config) EffectiveWhisperCliPath() string {
	if p := c.Settings.Get().WhisperCliPath; p != "" {
		return p
	}
	return c.WhisperCliPath
}

// EffectiveModelsDir returns the user-configured models directory override
// if one is set, else the conventional models/ default.
func (c *Config) EffectiveModelsDir() string {
	if d := c.Settings.Get().ModelsDir; d != "" {
		return d
	}
	return c.ModelsDir
}

// EffectiveDefaultVideoDir returns the user-configured directory the video
// picker should open to on load, or "" if none is set — there is no
// conventional default here (unlike the other Effective* methods), since
// the video picker falls back to listing drives when this is unset.
func (c *Config) EffectiveDefaultVideoDir() string {
	return c.Settings.Get().DefaultVideoDir
}

// CleanTmpDir removes any leftover files from a previous run that crashed
// before its own cleanup ran (internal/job's queue otherwise removes each
// job's temp WAV itself on both success and failure).
func (c *Config) CleanTmpDir() error {
	entries, err := os.ReadDir(c.TmpDir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := os.RemoveAll(filepath.Join(c.TmpDir, e.Name())); err != nil {
			return err
		}
	}
	return nil
}
