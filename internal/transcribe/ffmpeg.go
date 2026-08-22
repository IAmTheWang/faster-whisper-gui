package transcribe

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"faster-whisper-gui/internal/procutil"
)

// ExtractAudioArgs builds the ffmpeg argv (excluding the "ffmpeg" program
// name) that extracts and resamples videoPath's audio track into a 16kHz
// mono 16-bit PCM WAV file at outWavPath, overwriting any existing file.
// whisper.cpp requires exactly this format as input.
func ExtractAudioArgs(videoPath, outWavPath string) []string {
	return []string{
		"-y",
		"-i", videoPath,
		"-vn",
		"-ar", "16000",
		"-ac", "1",
		"-c:a", "pcm_s16le",
		outWavPath,
	}
}

// ExtractAudio runs ffmpeg to produce outWavPath from videoPath. The
// subprocess is bound to a Job Object so a canceled context can't leave an
// orphaned ffmpeg process behind. On failure, the returned error includes
// the tail of ffmpeg's stderr output.
func ExtractAudio(ctx context.Context, ffmpegPath, videoPath, outWavPath string) error {
	cmd := exec.CommandContext(ctx, ffmpegPath, ExtractAudioArgs(videoPath, outWavPath)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	mp, err := procutil.Start(cmd)
	if err != nil {
		return fmt.Errorf("start ffmpeg: %w", err)
	}
	defer mp.Close()

	if err := mp.Wait(); err != nil {
		return fmt.Errorf("ffmpeg failed: %w: %s", err, lastLines(stderr.String(), 20))
	}
	return nil
}

// ProbeDurationArgs builds the ffprobe argv that prints just the input's
// duration in seconds as a bare float to stdout (no headers, no extra
// formatting), so the caller can parse it directly.
func ProbeDurationArgs(videoPath string) []string {
	return []string{
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		videoPath,
	}
}

// ProbeDuration runs ffprobe and returns videoPath's duration.
func ProbeDuration(ctx context.Context, ffprobePath, videoPath string) (time.Duration, error) {
	cmd := exec.CommandContext(ctx, ffprobePath, ProbeDurationArgs(videoPath)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	mp, err := procutil.Start(cmd)
	if err != nil {
		return 0, fmt.Errorf("start ffprobe: %w", err)
	}
	defer mp.Close()

	if err := mp.Wait(); err != nil {
		return 0, fmt.Errorf("ffprobe failed: %w: %s", err, lastLines(stderr.String(), 20))
	}

	seconds, err := strconv.ParseFloat(strings.TrimSpace(stdout.String()), 64)
	if err != nil {
		return 0, fmt.Errorf("parse ffprobe duration %q: %w", stdout.String(), err)
	}
	return time.Duration(seconds * float64(time.Second)), nil
}

// lastLines returns at most n trailing lines of s, for trimming noisy
// subprocess output down to the part likely to explain a failure.
func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
