package procutil

import (
	"errors"
	"os/exec"
	"testing"
	"time"
)

// A real process can be frozen and thawed, and is still killable while frozen.
func TestSuspendResumeAndStopWhileSuspended(t *testing.T) {
	cmd := exec.Command("ping", "-n", "30", "127.0.0.1")
	mp, err := Start(cmd)
	if err != nil {
		t.Fatal(err)
	}
	defer mp.Close()

	if err := mp.Suspend(); err != nil {
		t.Fatalf("Suspend: %v", err)
	}
	if err := mp.Resume(); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if err := mp.Suspend(); err != nil {
		t.Fatalf("Suspend again: %v", err)
	}
	if err := mp.Stop(); err != nil {
		t.Fatalf("Stop while suspended: %v", err)
	}
	done := make(chan struct{})
	go func() { _ = mp.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("a suspended process was not terminated by Stop")
	}
}

func TestSuspendAfterCloseFails(t *testing.T) {
	cmd := exec.Command("ping", "-n", "1", "127.0.0.1")
	mp, err := Start(cmd)
	if err != nil {
		t.Fatal(err)
	}
	_ = mp.Wait()
	_ = mp.Close()
	if err := mp.Suspend(); !errors.Is(err, ErrProcessClosed) {
		t.Fatalf("Suspend after Close = %v, want ErrProcessClosed", err)
	}
}
