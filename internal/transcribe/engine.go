// Package transcribe drives the media -> audio -> SRT pipeline: extracting
// audio with ffmpeg from a video or audio source file, running a
// transcription engine (whisper.cpp today), and reporting progress as it
// goes.
package transcribe

import (
	"context"
	"time"
)

// Options describes a single, fully-resolved transcription request: every
// path is already absolute and cleaned, and TotalDuration has already been
// probed so the engine can turn segment timestamps into a percentage.
type Options struct {
	AudioPath     string        // 16kHz mono PCM WAV extracted from the source media file
	ModelPath     string        // absolute path to a ggml-*.bin model file
	Language      string        // language code, or "auto"
	OutputPrefix  string        // absolute path without extension; engine writes OutputPrefix+".srt"
	MaxLen        int           // max characters per subtitle cue; 0 = engine default
	MaxContext    int           // -mc passed to whisper-cli; 0 disables context carryover between segments (default, avoids hallucination propagating through the rest of a file); whisper-cli's own native default is -1 (unlimited)
	TotalDuration time.Duration // source audio duration, for progress percentage
}

// Progress is reported as transcription advances through the audio.
type Progress struct {
	Percent float64 // 0..100, best-effort estimate — not exact, inference speed isn't perfectly linear
	Message string  // human-readable status, e.g. the most recent segment's text
}

// Engine transcribes a WAV file to an SRT file written at
// Options.OutputPrefix + ".srt". Implementations wrap a specific backend —
// whisper.cpp today (see WhisperCppEngine) — so a future engine (e.g. a
// faster-whisper CLI subprocess) can be swapped in without touching callers
// that only depend on this interface.
type Engine interface {
	Transcribe(ctx context.Context, opts Options, onProgress func(Progress)) error
}
