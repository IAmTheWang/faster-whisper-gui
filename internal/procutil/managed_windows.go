package procutil

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
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
	exited bool          // Wait returned: the PID may be reused from now on
	done   chan struct{} // closed by Close; stops the StartContext watcher
}

// Start launches cmd and attempts to bind its process to a fresh job object.
// If job object creation or assignment fails — which can happen if this
// server is itself running inside a restrictive parent job/container — it
// logs a warning and degrades to relying on cmd.Process.Kill() alone; it
// does not treat that as a fatal error.
//
// The process is created suspended, put into the job, and only then resumed,
// so it cannot run — or spawn a child that would escape the job — before the
// assignment. It is always resumed, whether or not job tracking succeeded.
func Start(cmd *exec.Cmd) (*ManagedProcess, error) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_SUSPENDED
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	mp := &ManagedProcess{cmd: cmd, done: make(chan struct{})}
	mp.job = attachToJob(cmd)

	if err := resumeProcess(cmd.Process.Pid); err != nil {
		_ = cmd.Process.Kill()
		if mp.job != nil {
			_ = mp.job.Close()
		}
		_ = cmd.Wait()
		return nil, fmt.Errorf("resume process %d: %w", cmd.Process.Pid, err)
	}
	return mp, nil
}

// newJobObject is the job-object constructor attachToJob uses. It is a
// variable only so tests can force the degraded (no job object) path, which
// can't otherwise be provoked outside a restrictive parent job.
var newJobObject = NewJobObject

// attachToJob binds the (still suspended) process to a fresh job object, or
// returns nil after logging a warning when that is impossible.
func attachToJob(cmd *exec.Cmd) *JobObject {
	job, err := newJobObject()
	if err != nil {
		slog.Warn("procutil: job object unavailable, falling back to direct process kill", "error", err)
		return nil
	}
	if err := job.Assign(cmd); err != nil {
		slog.Warn("procutil: failed to assign process to job object, falling back to direct process kill", "pid", cmd.Process.Pid, "error", err)
		_ = job.Close()
		return nil
	}
	return job
}

var procNtResumeProcess = windows.NewLazySystemDLL("ntdll.dll").NewProc("NtResumeProcess")

// resumeProcess resumes a process created with CREATE_SUSPENDED. exec.Cmd does
// not expose the initial thread's handle, so the whole process is resumed.
func resumeProcess(pid int) error {
	h, err := windows.OpenProcess(windows.PROCESS_SUSPEND_RESUME, false, uint32(pid))
	if err != nil {
		return fmt.Errorf("open process: %w", err)
	}
	defer windows.CloseHandle(h)
	if status, _, _ := procNtResumeProcess.Call(uintptr(h)); status != 0 {
		return fmt.Errorf("NtResumeProcess: NTSTATUS %#x", status)
	}
	return nil
}

var procNtSuspendProcess = windows.NewLazySystemDLL("ntdll.dll").NewProc("NtSuspendProcess")

// setSuspended freezes (or thaws) every thread of the process. The process
// keeps its memory (including any GPU memory it holds) while suspended.
func setSuspended(pid int, suspend bool) error {
	h, err := windows.OpenProcess(windows.PROCESS_SUSPEND_RESUME, false, uint32(pid))
	if err != nil {
		return fmt.Errorf("open process: %w", err)
	}
	defer windows.CloseHandle(h)
	proc, name := procNtResumeProcess, "NtResumeProcess"
	if suspend {
		proc, name = procNtSuspendProcess, "NtSuspendProcess"
	}
	if status, _, _ := proc.Call(uintptr(h)); status != 0 {
		return fmt.Errorf("%s: NTSTATUS %#x", name, status)
	}
	return nil
}

// ErrProcessClosed is returned by Suspend/Resume once Close has been called:
// the process may be gone and its PID reused.
var ErrProcessClosed = fmt.Errorf("process is already closed")

// Suspend freezes the process until Resume. Not reference counted at this
// level: callers must not call it twice without a Resume in between.
func (mp *ManagedProcess) Suspend() error { return mp.suspend(true) }

// Resume thaws a process frozen by Suspend.
func (mp *ManagedProcess) Resume() error { return mp.suspend(false) }

func (mp *ManagedProcess) suspend(suspend bool) error {
	mp.mu.Lock()
	defer mp.mu.Unlock()
	if mp.closed || mp.exited || mp.cmd.Process == nil {
		return ErrProcessClosed
	}
	return setSuspended(mp.cmd.Process.Pid, suspend)
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
	err := mp.cmd.Wait()
	mp.mu.Lock()
	mp.exited = true
	mp.mu.Unlock()
	return err
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
