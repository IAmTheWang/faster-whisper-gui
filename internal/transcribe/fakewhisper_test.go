package transcribe

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// This file lets the tests run WhisperCppEngine and the health probes against
// a real subprocess without whisper-cli or ffmpeg installed: the test binary
// itself doubles as the fake. TestMain checks fakeWhisperEnv before the
// testing package parses flags (whisper-cli style args like "-m"/"-of" would
// otherwise be rejected as unknown test flags); the child inherits the
// variable because the tests set it with t.Setenv.
//
// Modes (the variable's value):
//
//	"1"    well-behaved run: three segments on stdout, a progress line on
//	       stderr, writes <-of prefix>.srt, exits 0
//	"fail" prints an error to stderr and exits 1
//	"hang" prints the first segment, then blocks (for cancellation tests)
const fakeWhisperEnv = "FWG_FAKE_WHISPER"

func TestMain(m *testing.M) {
	if mode := os.Getenv(fakeWhisperEnv); mode != "" {
		os.Exit(runFakeWhisper(mode, os.Args[1:]))
	}
	os.Exit(m.Run())
}

// fakeSegments are the per-segment stdout lines the fake prints, in
// whisper-cli's format. Their end timestamps (2s, 4s, 6s) are what the engine
// turns into a percentage.
var fakeSegments = []string{
	"[00:00:00.000 --> 00:00:02.000]   Hello",
	"[00:00:02.000 --> 00:00:04.000]   there",
	"[00:00:04.000 --> 00:00:06.000]   Goodbye",
}

func runFakeWhisper(mode string, args []string) int {
	// Health probes run "<tool> --help" / "<tool> -version".
	for _, a := range args {
		if a == "--help" || a == "-version" {
			if mode == "fail" {
				fmt.Fprintln(os.Stderr, "boom: probe failed")
				return 1
			}
			fmt.Println("fake-tool 1.2.3")
			fmt.Println("second line that must not reach the health detail")
			return 0
		}
	}

	var prefix string
	for i, a := range args {
		if a == "-of" && i+1 < len(args) {
			prefix = args[i+1]
		}
	}

	switch mode {
	case "fail":
		fmt.Fprintln(os.Stderr, "whisper_init_from_file: loading model")
		fmt.Fprintln(os.Stderr, "boom: unsupported model format")
		return 1
	case "hang":
		fmt.Println(fakeSegments[0])
		time.Sleep(time.Minute)
		return 0
	}

	for _, line := range fakeSegments {
		fmt.Println(line)
	}
	fmt.Fprintln(os.Stderr, "whisper_print_progress_callback: progress =  50%")
	if prefix != "" {
		srt := "1\n00:00:00,000 --> 00:00:02,000\nHello\n\n"
		if err := os.WriteFile(prefix+".srt", []byte(srt), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "write srt:", err)
			return 2
		}
		// Let the test verify exactly which argv the engine passed.
		if err := os.WriteFile(prefix+".args", []byte(strings.Join(args, "\n")), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "write args:", err)
			return 2
		}
	}
	return 0
}

// fakeEngine returns a WhisperCppEngine whose "whisper-cli" is this test
// binary running in the given fake mode.
func fakeEngine(t *testing.T, mode string) *WhisperCppEngine {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(fakeWhisperEnv, mode)
	return &WhisperCppEngine{CliPath: func() string { return exe }}
}

func fakeOptions(t *testing.T, total time.Duration) Options {
	t.Helper()
	return Options{
		AudioPath:     filepath.Join(t.TempDir(), "in.wav"),
		ModelPath:     filepath.Join(t.TempDir(), "ggml-tiny.bin"),
		Language:      "en",
		OutputPrefix:  filepath.Join(t.TempDir(), "out"),
		MaxLen:        40,
		MaxContext:    0,
		TotalDuration: total,
	}
}

func TestWhisperCppEngine_Transcribe_ProgressFromStdoutTimestamps(t *testing.T) {
	eng := fakeEngine(t, "1")
	opts := fakeOptions(t, 10*time.Second)

	var got []Progress
	if err := eng.Transcribe(context.Background(), opts, func(p Progress) { got = append(got, p) }); err != nil {
		t.Fatalf("Transcribe: %v", err)
	}

	if len(got) == 0 {
		t.Fatal("onProgress never called")
	}
	for i := 1; i < len(got); i++ {
		if got[i].Percent < got[i-1].Percent {
			t.Fatalf("progress went backwards at %d: %v -> %v (all: %v)", i, got[i-1].Percent, got[i].Percent, got)
		}
	}
	// The stderr line says 50%, so a final 60% can only come from the last
	// segment's end timestamp (6s of 10s).
	last := got[len(got)-1]
	if last.Percent != 60 {
		t.Errorf("final percent = %v, want 60 (6s segment end / 10s total)", last.Percent)
	}
	if last.Message != "Goodbye" {
		t.Errorf("final message = %q, want the last segment's text", last.Message)
	}

	srt, err := os.ReadFile(opts.OutputPrefix + ".srt")
	if err != nil {
		t.Fatalf("expected the engine to leave %s.srt: %v", opts.OutputPrefix, err)
	}
	if !strings.Contains(string(srt), "Hello") {
		t.Errorf("srt = %q, want it to contain the fake's cue", srt)
	}

	rawArgs, err := os.ReadFile(opts.OutputPrefix + ".args")
	if err != nil {
		t.Fatal(err)
	}
	if want := strings.Join(BuildWhisperArgs(opts), "\n"); string(rawArgs) != want {
		t.Errorf("child argv =\n%s\nwant BuildWhisperArgs:\n%s", rawArgs, want)
	}
}

func TestWhisperCppEngine_Transcribe_ProgressIsCappedAt100(t *testing.T) {
	eng := fakeEngine(t, "1")
	// The last segment ends at 6s, past this 3s "probed" duration.
	opts := fakeOptions(t, 3*time.Second)

	var last Progress
	if err := eng.Transcribe(context.Background(), opts, func(p Progress) { last = p }); err != nil {
		t.Fatalf("Transcribe: %v", err)
	}
	if last.Percent != 100 {
		t.Errorf("final percent = %v, want 100 (capped)", last.Percent)
	}
}

// With no usable total duration the stdout timestamps cannot be turned into a
// percentage, so whisper-cli's own stderr percentage is the only signal.
func TestWhisperCppEngine_Transcribe_StderrFallbackWithoutDuration(t *testing.T) {
	eng := fakeEngine(t, "1")
	opts := fakeOptions(t, 0)

	var got []Progress
	if err := eng.Transcribe(context.Background(), opts, func(p Progress) { got = append(got, p) }); err != nil {
		t.Fatalf("Transcribe: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("onProgress never called; the stderr percentage should still be reported")
	}
	if last := got[len(got)-1]; last.Percent != 50 {
		t.Errorf("final percent = %v, want 50 from the stderr progress line", last.Percent)
	}
	if last := got[len(got)-1]; last.Message == "" {
		t.Error("segment text was dropped; it should still be shown without a probed duration")
	}
}

func TestWhisperCppEngine_Transcribe_NilCallbackIsFine(t *testing.T) {
	eng := fakeEngine(t, "1")
	opts := fakeOptions(t, 10*time.Second)
	if err := eng.Transcribe(context.Background(), opts, nil); err != nil {
		t.Fatalf("Transcribe with nil onProgress: %v", err)
	}
	if _, err := os.Stat(opts.OutputPrefix + ".srt"); err != nil {
		t.Errorf("srt not written: %v", err)
	}
}

func TestWhisperCppEngine_Transcribe_FailureCarriesStderrTail(t *testing.T) {
	eng := fakeEngine(t, "fail")
	opts := fakeOptions(t, 10*time.Second)

	err := eng.Transcribe(context.Background(), opts, nil)
	if err == nil {
		t.Fatal("Transcribe succeeded, want an error from the failing whisper-cli")
	}
	msg := err.Error()
	for _, want := range []string{"whisper-cli failed", "exit status 1", "boom: unsupported model format", "loading model"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q does not contain %q", msg, want)
		}
	}
	if _, statErr := os.Stat(opts.OutputPrefix + ".srt"); statErr == nil {
		t.Error("a failed run must not leave a .srt behind")
	}
}

func TestWhisperCppEngine_Transcribe_MissingBinary(t *testing.T) {
	eng := &WhisperCppEngine{CliPath: func() string { return filepath.Join(t.TempDir(), "no-such-whisper-cli.exe") }}
	err := eng.Transcribe(context.Background(), fakeOptions(t, time.Second), nil)
	if err == nil || !strings.Contains(err.Error(), "start whisper-cli") {
		t.Fatalf("error = %v, want a \"start whisper-cli\" failure", err)
	}
}

// runTranscribe runs Transcribe in a goroutine and fails the test if it has
// not returned within limit, instead of hanging the whole package.
func runTranscribe(t *testing.T, ctx context.Context, eng *WhisperCppEngine, opts Options, onProgress func(Progress), limit time.Duration) (time.Duration, error) {
	t.Helper()
	start := time.Now()
	done := make(chan error, 1)
	go func() { done <- eng.Transcribe(ctx, opts, onProgress) }()
	select {
	case err := <-done:
		return time.Since(start), err
	case <-time.After(limit):
		t.Fatalf("Transcribe still running %v after cancel", limit)
		return 0, nil
	}
}

func TestWhisperCppEngine_Transcribe_CancelReturnsPromptly(t *testing.T) {
	eng := fakeEngine(t, "hang")
	opts := fakeOptions(t, 10*time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Cancel as soon as the child has demonstrably started producing output.
	onProgress := func(Progress) { cancel() }

	elapsed, err := runTranscribe(t, ctx, eng, opts, onProgress, 10*time.Second)
	if err == nil {
		t.Fatal("Transcribe returned nil after cancel, want an error")
	}
	if !strings.Contains(err.Error(), "whisper-cli failed") {
		t.Errorf("error = %q, want a whisper-cli failure", err)
	}
	if elapsed > 5*time.Second {
		t.Errorf("Transcribe took %v to return after cancel", elapsed)
	}
}

func TestWhisperCppEngine_Transcribe_AlreadyCanceledContext(t *testing.T) {
	eng := fakeEngine(t, "hang")
	opts := fakeOptions(t, 10*time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	elapsed, err := runTranscribe(t, ctx, eng, opts, nil, 10*time.Second)
	if err == nil {
		t.Fatal("Transcribe returned nil for a canceled context, want an error")
	}
	if elapsed > 5*time.Second {
		t.Errorf("Transcribe took %v with an already-canceled context", elapsed)
	}
}

func TestCheckWhisperCliAndFfmpeg_BadPath(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope.exe")
	checks := map[string]func(context.Context, string) ComponentHealth{
		"CheckWhisperCli": CheckWhisperCli,
		"CheckFfmpeg":     CheckFfmpeg,
	}
	for name, check := range checks {
		t.Run(name, func(t *testing.T) {
			h := check(context.Background(), missing)
			if h.OK {
				t.Fatal("OK = true for a binary that does not exist")
			}
			if strings.TrimSpace(h.Detail) == "" {
				t.Error("Detail is empty; the frontend needs a message to show")
			}
		})
	}
}

func TestCheckWhisperCliAndFfmpeg_WorkingBinary(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(fakeWhisperEnv, "1")
	checks := map[string]func(context.Context, string) ComponentHealth{
		"CheckWhisperCli": CheckWhisperCli,
		"CheckFfmpeg":     CheckFfmpeg,
	}
	for name, check := range checks {
		t.Run(name, func(t *testing.T) {
			h := check(context.Background(), exe)
			if !h.OK {
				t.Fatalf("OK = false: %s", h.Detail)
			}
			// Only the first output line (the version banner) is surfaced.
			if h.Detail != "fake-tool 1.2.3" {
				t.Errorf("Detail = %q, want the first output line only", h.Detail)
			}
		})
	}
}

func TestCheckWhisperCliAndFfmpeg_NonZeroExitSurfacesOutput(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(fakeWhisperEnv, "fail")
	checks := map[string]func(context.Context, string) ComponentHealth{
		"CheckWhisperCli": CheckWhisperCli,
		"CheckFfmpeg":     CheckFfmpeg,
	}
	for name, check := range checks {
		t.Run(name, func(t *testing.T) {
			h := check(context.Background(), exe)
			if h.OK {
				t.Fatal("OK = true for a binary that exits non-zero")
			}
			if !strings.Contains(h.Detail, "boom: probe failed") {
				t.Errorf("Detail = %q, want the binary's own output", h.Detail)
			}
		})
	}
}

// The controller handed out through OnStart really freezes the subprocess; a
// frozen one is still canceled promptly, and the handle is withdrawn (nil) when
// Transcribe returns.
func TestWhisperCppEngine_Transcribe_ControllerSuspendResumeThenCancel(t *testing.T) {
	eng := fakeEngine(t, "hang")
	opts := fakeOptions(t, 10*time.Second)

	var (
		mu        sync.Mutex
		ctl       Controller
		withdrawn bool
	)
	opts.OnStart = func(c Controller) {
		mu.Lock()
		defer mu.Unlock()
		if c == nil {
			withdrawn = true
			return
		}
		ctl = c
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			mu.Lock()
			c := ctl
			mu.Unlock()
			if c != nil {
				if err := c.Suspend(); err != nil {
					t.Errorf("Suspend: %v", err)
				} else if err := c.Resume(); err != nil {
					t.Errorf("Resume: %v", err)
				} else if err := c.Suspend(); err != nil {
					t.Errorf("Suspend again: %v", err)
				}
				cancel() // canceled while frozen
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		cancel()
	}()

	if _, err := runTranscribe(t, ctx, eng, opts, nil, 10*time.Second); err == nil {
		t.Fatal("want an error after cancel")
	}
	mu.Lock()
	defer mu.Unlock()
	if ctl == nil || !withdrawn {
		t.Fatalf("controller handed out=%v, withdrawn=%v; want both true", ctl != nil, withdrawn)
	}
}
