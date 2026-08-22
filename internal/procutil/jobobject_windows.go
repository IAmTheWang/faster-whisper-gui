package procutil

import (
	"fmt"
	"os/exec"
	"unsafe"

	"golang.org/x/sys/windows"
)

// JobObject wraps a Windows Job Object configured with
// JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE: every process assigned to it is killed
// as soon as the job handle closes, including when this program exits
// unexpectedly. This is what keeps ffmpeg/whisper-cli (and any children they
// spawn) from surviving as orphaned processes after a cancel or a crash —
// plain cmd.Process.Kill() only ever reaches the direct child.
type JobObject struct {
	handle windows.Handle
}

// NewJobObject creates a new unnamed job object with kill-on-close semantics.
func NewJobObject() (*JobObject, error) {
	handle, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, fmt.Errorf("create job object: %w", err)
	}

	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
			LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
		},
	}
	_, err = windows.SetInformationJobObject(
		handle,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	)
	if err != nil {
		_ = windows.CloseHandle(handle)
		return nil, fmt.Errorf("set job object limits: %w", err)
	}

	return &JobObject{handle: handle}, nil
}

// Assign binds an already-started command's process to the job. Call this
// immediately after cmd.Start().
//
// This can fail in environments where the server process itself runs inside
// a restrictive parent job that denies nesting (e.g. some sandboxed
// launchers) — callers should treat a non-nil error as "job-object cleanup
// unavailable" (log a warning and fall back to cmd.Process.Kill() for direct
// cancellation) rather than as fatal.
func (j *JobObject) Assign(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return fmt.Errorf("assign to job object: process not started")
	}

	procHandle, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
	if err != nil {
		return fmt.Errorf("open process %d: %w", cmd.Process.Pid, err)
	}
	defer windows.CloseHandle(procHandle)

	if err := windows.AssignProcessToJobObject(j.handle, procHandle); err != nil {
		return fmt.Errorf("assign process %d to job object: %w", cmd.Process.Pid, err)
	}
	return nil
}

// Terminate kills every process currently assigned to the job.
func (j *JobObject) Terminate() error {
	return windows.TerminateJobObject(j.handle, 1)
}

// Close releases the job object handle. Any processes still assigned to it
// are killed as a side effect (JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE) — this is
// the safety net that covers server crashes/unexpected exits, not just
// explicit cancellation.
func (j *JobObject) Close() error {
	return windows.CloseHandle(j.handle)
}
