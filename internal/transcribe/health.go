package transcribe

import (
	"bytes"
	"context"
	"strings"
	"time"

	"os/exec"

	"faster-whisper-gui/internal/procutil"
)

// ComponentHealth is the result of probing an external binary dependency
// (ffmpeg or whisper-cli) so problems (missing file, missing DLL, wrong
// build) surface at startup/page-load instead of after a user submits a
// real job.
type ComponentHealth struct {
	OK     bool   `json:"ok"`
	Detail string `json:"detail"` // version string when OK, error message otherwise
}

const healthCheckTimeout = 5 * time.Second

func probeExecutable(ctx context.Context, path string, args ...string) ComponentHealth {
	ctx, cancel := context.WithTimeout(ctx, healthCheckTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, path, args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	mp, err := procutil.Start(cmd)
	if err != nil {
		return ComponentHealth{OK: false, Detail: err.Error()}
	}
	defer mp.Close()

	if err := mp.Wait(); err != nil {
		detail := strings.TrimSpace(lastLines(out.String(), 5))
		if detail == "" {
			detail = err.Error()
		}
		return ComponentHealth{OK: false, Detail: detail}
	}

	detail := strings.TrimSpace(out.String())
	if idx := strings.IndexByte(detail, '\n'); idx >= 0 {
		detail = detail[:idx]
	}
	return ComponentHealth{OK: true, Detail: detail}
}

// CheckFfmpeg probes ffmpegPath by running it with -version.
func CheckFfmpeg(ctx context.Context, ffmpegPath string) ComponentHealth {
	return probeExecutable(ctx, ffmpegPath, "-version")
}

// CheckWhisperCli probes whisperCliPath by running it with --help.
func CheckWhisperCli(ctx context.Context, whisperCliPath string) ComponentHealth {
	return probeExecutable(ctx, whisperCliPath, "--help")
}
