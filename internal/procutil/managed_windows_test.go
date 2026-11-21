package procutil

import (
	"bytes"
	"context"
	"os/exec"
	"testing"
	"time"
)

// A canceled context must end Wait promptly even when a grandchild (ping,
// spawned by cmd) still holds the stdout pipe — the case exec.CommandContext
// alone does not handle.
func TestStartContext_CancelKillsGrandchildren(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.Command("cmd", "/c", "ping -n 60 127.0.0.1")
	var out bytes.Buffer
	cmd.Stdout = &out

	mp, err := StartContext(ctx, cmd)
	if err != nil {
		t.Fatal(err)
	}
	defer mp.Close()

	waited := make(chan error, 1)
	go func() { waited <- mp.Wait() }()

	time.Sleep(500 * time.Millisecond)
	cancel()

	select {
	case err := <-waited:
		if err == nil {
			t.Fatal("Wait returned nil after cancel, want an error")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Wait still blocked 3s after cancel; grandchild was not terminated")
	}
}

func TestManagedProcess_CloseIsIdempotentAndStopAfterCloseIsNoop(t *testing.T) {
	cmd := exec.Command("cmd", "/c", "exit 0")
	mp, err := Start(cmd)
	if err != nil {
		t.Fatal(err)
	}
	_ = mp.Wait()
	if err := mp.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := mp.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if err := mp.Stop(); err != nil {
		t.Fatalf("Stop after Close: %v", err)
	}
}

// Creating the process suspended must not leave it frozen: it has to be
// resumed and run to completion with its output intact.
func TestStart_ResumesSuspendedProcess(t *testing.T) {
	cmd := exec.Command("cmd", "/c", "echo hello & exit 3")
	var out bytes.Buffer
	cmd.Stdout = &out
	mp, err := Start(cmd)
	if err != nil {
		t.Fatal(err)
	}
	defer mp.Close()

	done := make(chan error, 1)
	go func() { done <- mp.Wait() }()
	select {
	case err := <-done:
		if ee, ok := err.(*exec.ExitError); !ok || ee.ExitCode() != 3 {
			t.Fatalf("Wait = %v, want exit code 3", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("process never ran: still suspended?")
	}
	if got := bytes.TrimSpace(out.Bytes()); string(got) != "hello" {
		t.Fatalf("stdout = %q, want hello", got)
	}
}
