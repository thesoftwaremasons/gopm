//go:build windows

// Package process holds platform-specific helpers for spawning and killing
// managed child processes. On Windows we put each child in its own process
// group and tear down its descendants via taskkill /T (a pure-Go Job Objects
// implementation can replace this in a later phase).
package process

import (
	"os/exec"
	"strconv"
	"syscall"
)

// SetSysProcAttr configures cmd to start in a new process group so we can
// later signal the whole tree.
func SetSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
}

// Kill terminates the child and its descendants. On Windows there is no
// SIGTERM/SIGKILL distinction; force currently has no effect beyond using
// /F. We shell out to taskkill since it works on every supported Windows
// version and stays CGo-free.
func Kill(cmd *exec.Cmd, force bool) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	pid := cmd.Process.Pid
	args := []string{"/T"}
	if force {
		args = append(args, "/F")
	}
	args = append(args, "/PID", strconv.Itoa(pid))
	tk := exec.Command("taskkill", args...)
	return tk.Run()
}
