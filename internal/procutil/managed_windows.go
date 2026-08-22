package procutil

import (
	"log/slog"
	"os/exec"
)

// ManagedProcess is a subprocess whose lifetime is tied to a Windows Job
// Object, so cancellation — or this server crashing — cannot leave it, or
// any child process it spawns, running as an orphan.
type ManagedProcess struct {
	cmd *exec.Cmd
	job *JobObject // nil in degraded mode (job object unavailable)
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

	mp := &ManagedProcess{cmd: cmd}

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

// Wait blocks until the process exits.
func (mp *ManagedProcess) Wait() error {
	return mp.cmd.Wait()
}

// Stop forcibly terminates the process and, when job-object tracking is
// active, every process it spawned.
func (mp *ManagedProcess) Stop() error {
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
// it — safe to call unconditionally after Wait returns.
func (mp *ManagedProcess) Close() error {
	if mp.job != nil {
		return mp.job.Close()
	}
	return nil
}
