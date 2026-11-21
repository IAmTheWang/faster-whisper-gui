package procutil

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// forceNewJobObject swaps the job-object constructor for the duration of the
// test. Tests using it must not run in parallel.
func forceNewJobObject(t *testing.T, fn func() (*JobObject, error)) {
	t.Helper()
	orig := newJobObject
	newJobObject = fn
	t.Cleanup(func() { newJobObject = orig })
}

// captureLogs redirects slog's default logger into the returned buffer.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	orig := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(orig) })
	return &buf
}

func failingJobObject() (*JobObject, error) {
	return nil, errors.New("job objects are not available here")
}

// An unusable job-object API (e.g. a restrictive parent job) must degrade to
// plain process handling: the process is still resumed and runs to completion
// with its output and exit code intact.
func TestStart_DegradedModeStillRunsProcess(t *testing.T) {
	forceNewJobObject(t, failingJobObject)
	logs := captureLogs(t)

	cmd := exec.Command("cmd", "/c", "echo hello & exit 3")
	var out bytes.Buffer
	cmd.Stdout = &out
	mp, err := Start(cmd)
	if err != nil {
		t.Fatalf("Start in degraded mode: %v", err)
	}
	defer mp.Close()

	if mp.job != nil {
		t.Error("mp.job != nil, want nil when job object creation failed")
	}

	done := make(chan error, 1)
	go func() { done <- mp.Wait() }()
	select {
	case err := <-done:
		var ee *exec.ExitError
		if !errors.As(err, &ee) || ee.ExitCode() != 3 {
			t.Fatalf("Wait = %v, want exit code 3", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("process never ran to completion in degraded mode: still suspended?")
	}
	if got := strings.TrimSpace(out.String()); got != "hello" {
		t.Errorf("stdout = %q, want hello", got)
	}
	if !strings.Contains(logs.String(), "job object unavailable") {
		t.Errorf("expected a warning about the missing job object, logs = %q", logs.String())
	}

	if err := mp.Close(); err != nil {
		t.Errorf("Close in degraded mode: %v", err)
	}
	if err := mp.Close(); err != nil {
		t.Errorf("second Close in degraded mode: %v", err)
	}
	if err := mp.Stop(); err != nil {
		t.Errorf("Stop after Close in degraded mode: %v", err)
	}
}

// In degraded mode Stop has no job to terminate, so it must kill the direct
// child with Process.Kill. ping is a single process (no grandchild), so a
// plain kill is enough to end it.
func TestStop_DegradedModeKillsProcess(t *testing.T) {
	forceNewJobObject(t, failingJobObject)
	captureLogs(t)

	mp, err := Start(exec.Command("ping", "-n", "60", "127.0.0.1"))
	if err != nil {
		t.Fatalf("Start in degraded mode: %v", err)
	}
	defer mp.Close()
	if mp.job != nil {
		t.Fatal("mp.job != nil, want nil in degraded mode")
	}

	waited := make(chan error, 1)
	go func() { waited <- mp.Wait() }()

	select {
	case err := <-waited:
		t.Fatalf("process exited on its own before Stop: %v", err)
	case <-time.After(300 * time.Millisecond):
	}

	if err := mp.Stop(); err != nil {
		t.Fatalf("Stop in degraded mode: %v", err)
	}
	select {
	case err := <-waited:
		if err == nil {
			t.Fatal("Wait returned nil after Stop, want an error from the killed process")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("process still running 3s after Stop in degraded mode")
	}

	if err := mp.Close(); err != nil {
		t.Errorf("Close after Stop: %v", err)
	}
	if err := mp.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
	if err := mp.Stop(); err != nil {
		t.Errorf("Stop after Close: %v", err)
	}
}

// StartContext's cancellation watcher goes through Stop, so it has to work in
// degraded mode too.
func TestStartContext_DegradedModeCancelKillsProcess(t *testing.T) {
	forceNewJobObject(t, failingJobObject)
	captureLogs(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mp, err := StartContext(ctx, exec.Command("ping", "-n", "60", "127.0.0.1"))
	if err != nil {
		t.Fatal(err)
	}
	defer mp.Close()

	waited := make(chan error, 1)
	go func() { waited <- mp.Wait() }()

	time.Sleep(200 * time.Millisecond)
	cancel()

	select {
	case err := <-waited:
		if err == nil {
			t.Fatal("Wait returned nil after cancel, want an error")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("process still running 3s after cancel in degraded mode")
	}
}

// A job object that exists but rejects the assignment (here a zero handle,
// which is never valid) must also degrade rather than fail Start, and the
// half-built job must not be kept.
func TestStart_DegradedWhenAssignFails(t *testing.T) {
	forceNewJobObject(t, func() (*JobObject, error) { return &JobObject{}, nil })
	logs := captureLogs(t)

	cmd := exec.Command("cmd", "/c", "exit 0")
	mp, err := Start(cmd)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer mp.Close()
	if mp.job != nil {
		t.Error("mp.job != nil, want nil after a failed assignment")
	}
	if err := mp.Wait(); err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if !strings.Contains(logs.String(), "failed to assign process to job object") {
		t.Errorf("expected an assignment-failure warning, logs = %q", logs.String())
	}
}

// Sanity check for the non-degraded default: the injected constructor is the
// real one and a started process is tracked by a job object.
func TestStart_AttachesJobObjectByDefault(t *testing.T) {
	mp, err := Start(exec.Command("cmd", "/c", "exit 0"))
	if err != nil {
		t.Fatal(err)
	}
	defer mp.Close()
	if mp.job == nil {
		t.Skip("job object unavailable in this environment (nested in a restrictive job?)")
	}
	if err := mp.Wait(); err != nil {
		t.Fatalf("Wait: %v", err)
	}
}
