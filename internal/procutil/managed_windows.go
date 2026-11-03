package procutil

import (
	"context"
	"log/slog"
	"os/exec"
	"sync"
	"time"
)

// cancelWaitDelay bounds how long Wait blocks on I/O pipes after the process
// has been killed, in case something outside the job still holds them open.
const cancelWaitDelay = 5 * time.Second

// ManagedProcess is a subprocess whose lifetime is tied to a Windows Job
// Object, so cancellation — or this server crashing — cannot leave it, or
// any child process it spawns, running as an orphan.
type ManagedProcess struct {
	cmd *exec.Cmd

	mu     sync.Mutex
	job    *JobObject // nil in degraded mode (job object unavailable) or once closed
	closed bool
	done   chan struct{} // closed by Close; stops the StartContext watcher
}

// Start launches cmd and attempts to bind its process to a fresh job object.
// If job object creation or assignment fails — which can happen if this
// server is itself running inside a restrictive parent job/container — it
// logs a warning and degrades to relying on cmd.Process.Kill() alone; it
// does not treat that as a fatal error.
func Start(cmd *exec.Cmd) (*ManagedProcess, error) {
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	mp := &ManagedProcess{cmd: cmd, done: make(chan struct{})}

	job, err := NewJobObject()
	if err != nil {
		slog.Warn("procutil: job object unavailable, falling back to direct process kill", "error", err)
		return mp, nil
	}

	if err := job.Assign(cmd); err != nil {
		slog.Warn("procutil: failed to assign process to job object, falling back to direct process kill", "pid", cmd.Process.Pid, "error", err)
		_ = job.Close()
		return mp, nil
	}

	mp.job = job
	return mp, nil
}

// StartContext is Start plus context cancellation: when ctx is done the whole
// job object tree is terminated, not just the direct child. Plain
// exec.CommandContext only kills the direct child, so a grandchild still
// holding the stdout/stderr pipes would leave Wait blocked indefinitely.
// cmd.WaitDelay is also set (if unset) as a backstop for degraded mode.
// Callers must still call Close once done with the process.
func StartContext(ctx context.Context, cmd *exec.Cmd) (*ManagedProcess, error) {
	if cmd.WaitDelay == 0 {
		cmd.WaitDelay = cancelWaitDelay
	}
	mp, err := Start(cmd)
	if err != nil {
		return nil, err
	}
	go func() {
		select {
		case <-ctx.Done():
			_ = mp.Stop()
		case <-mp.done:
		}
	}()
	return mp, nil
}

// Wait blocks until the process exits.
func (mp *ManagedProcess) Wait() error {
	return mp.cmd.Wait()
}

// Stop forcibly terminates the process and, when job-object tracking is
// active, every process it spawned. It is a no-op after Close.
func (mp *ManagedProcess) Stop() error {
	mp.mu.Lock()
	defer mp.mu.Unlock()
	if mp.closed {
		return nil
	}
	if mp.job != nil {
		return mp.job.Terminate()
	}
	if mp.cmd.Process != nil {
		return mp.cmd.Process.Kill()
	}
	return nil
}

// Close releases the job object, if any. Because the job was created with
// kill-on-close semantics, this also kills any processes still running under
// it — safe to call unconditionally after Wait returns, and idempotent.
func (mp *ManagedProcess) Close() error {
	mp.mu.Lock()
	defer mp.mu.Unlock()
	if mp.closed {
		return nil
	}
	mp.closed = true
	close(mp.done)
	if mp.job != nil {
		job := mp.job
		mp.job = nil
		return job.Close()
	}
	return nil
}
