package transcribe

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"faster-whisper-gui/internal/procutil"
)

// BuildWhisperArgs builds the whisper-cli argv (excluding the program name)
// for the given options. Language defaults to "auto" if unset.
func BuildWhisperArgs(opts Options) []string {
	lang := opts.Language
	if lang == "" {
		lang = "auto"
	}

	args := []string{
		"-m", opts.ModelPath,
		"-f", opts.AudioPath,
		"-l", lang,
		"-of", opts.OutputPrefix,
		"-osrt",
	}
	if opts.MaxLen > 0 {
		args = append(args, "-ml", strconv.Itoa(opts.MaxLen))
	}
	return args
}

// WhisperCppEngine implements Engine by shelling out to a whisper-cli.exe
// binary (subprocess, not cgo — see the architecture notes in the project
// plan for why).
type WhisperCppEngine struct {
	// CliPath is called fresh for every Transcribe invocation rather than
	// stored as a plain string, because a single WhisperCppEngine instance
	// is constructed once and reused for every job (see internal/appserver)
	// — a plain string field would freeze in whatever path was configured
	// at startup, ignoring later Settings-panel changes. Typically a
	// (*config.Config).EffectiveWhisperCliPath method value.
	CliPath func() string
}

var _ Engine = (*WhisperCppEngine)(nil)

// segmentLineRe matches whisper-cli's per-segment stdout lines, e.g.:
//
//	[00:00:12.340 --> 00:00:15.560]   Hello there.
//
// The end timestamp is the primary signal used to compute progress, since
// it's what most directly reflects how far into the audio transcription has
// reached (whisper.cpp's own stderr percentage has been reported buggy on
// long files — see the project plan's research notes).
var segmentLineRe = regexp.MustCompile(`\[\d{2}:\d{2}:\d{2}\.\d{3}\s*-->\s*(\d{2}):(\d{2}):(\d{2})\.(\d{3})\]\s*(.*)`)

// stderrPercentRe matches whisper-cli's own coarse progress line, e.g.:
//
//	whisper_print_progress_callback: progress =  42%
//
// Used only as a secondary/supplementary signal.
var stderrPercentRe = regexp.MustCompile(`progress\s*=\s*(\d+)%`)

func parseSegmentEnd(line string) (end time.Duration, text string, ok bool) {
	m := segmentLineRe.FindStringSubmatch(line)
	if m == nil {
		return 0, "", false
	}
	h, _ := strconv.Atoi(m[1])
	mi, _ := strconv.Atoi(m[2])
	s, _ := strconv.Atoi(m[3])
	ms, _ := strconv.Atoi(m[4])
	end = time.Duration(h)*time.Hour +
		time.Duration(mi)*time.Minute +
		time.Duration(s)*time.Second +
		time.Duration(ms)*time.Millisecond
	return end, strings.TrimSpace(m[5]), true
}

func parseStderrPercent(line string) (float64, bool) {
	m := stderrPercentRe.FindStringSubmatch(line)
	if m == nil {
		return 0, false
	}
	v, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

type progressUpdate struct {
	percent float64
	message string
}

// Transcribe runs whisper-cli against opts.AudioPath and writes
// opts.OutputPrefix + ".srt". Progress is derived primarily from stdout
// segment timestamps (divided by opts.TotalDuration) and secondarily from
// whisper-cli's own stderr percentage line; onProgress is called with the
// running best (monotonically non-decreasing) estimate.
func (e *WhisperCppEngine) Transcribe(ctx context.Context, opts Options, onProgress func(Progress)) error {
	cmd := exec.CommandContext(ctx, e.CliPath(), BuildWhisperArgs(opts)...)

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("whisper-cli stdout pipe: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("whisper-cli stderr pipe: %w", err)
	}

	mp, err := procutil.Start(cmd)
	if err != nil {
		return fmt.Errorf("start whisper-cli: %w", err)
	}
	defer mp.Close()

	const maxTailLines = 20
	var stderrTail []string

	updates := make(chan progressUpdate, 16)
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		scanner := procutil.NewCRLineScanner(stdoutPipe)
		for scanner.Scan() {
			line := scanner.Text()
			if opts.TotalDuration <= 0 {
				continue
			}
			if end, text, ok := parseSegmentEnd(line); ok {
				pct := float64(end) / float64(opts.TotalDuration) * 100
				if pct > 100 {
					pct = 100
				}
				updates <- progressUpdate{percent: pct, message: text}
			}
		}
	}()

	go func() {
		defer wg.Done()
		scanner := procutil.NewCRLineScanner(stderrPipe)
		for scanner.Scan() {
			line := scanner.Text()
			stderrTail = append(stderrTail, line)
			if len(stderrTail) > maxTailLines {
				stderrTail = stderrTail[len(stderrTail)-maxTailLines:]
			}
			if pct, ok := parseStderrPercent(line); ok {
				updates <- progressUpdate{percent: pct}
			}
		}
	}()

	go func() {
		wg.Wait()
		close(updates)
	}()

	var best float64
	var lastMessage string
	for u := range updates {
		if u.percent > best {
			best = u.percent
		}
		if u.message != "" {
			lastMessage = u.message
		}
		if onProgress != nil {
			onProgress(Progress{Percent: best, Message: lastMessage})
		}
	}

	if err := mp.Wait(); err != nil {
		return fmt.Errorf("whisper-cli failed: %w: %s", err, strings.Join(stderrTail, "\n"))
	}
	return nil
}
