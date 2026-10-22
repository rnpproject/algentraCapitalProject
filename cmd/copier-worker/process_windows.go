//go:build windows

package main

import (
	"os"
	"os/exec"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

func prepareChild(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x00000200} // CREATE_NEW_PROCESS_GROUP
}

func stopChildProcess(cmd *exec.Cmd, done <-chan error, timeout time.Duration) error {
	select {
	case err := <-done:
		return err
	default:
	}
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	// CTRL_BREAK_EVENT gives the terminal a graceful stop before the supervisor
	// falls back to TerminateProcess.
	if err := windows.GenerateConsoleCtrlEvent(windows.CTRL_BREAK_EVENT, uint32(cmd.Process.Pid)); err != nil {
		_ = cmd.Process.Kill()
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case err := <-done:
		return err
	case <-timer.C:
		_ = cmd.Process.Kill()
		select {
		case err := <-done:
			return err
		case <-time.After(5 * time.Second):
			return os.ErrDeadlineExceeded
		}
	}
}
