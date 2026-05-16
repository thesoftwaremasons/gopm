//go:build linux || darwin

// Package process holds platform-specific helpers for spawning and killing
// managed child processes. On Unix we run each child in its own process
// group so the entire tree can be signalled at once.
package process

import (
	"os/exec"
	"syscall"
)

// SetSysProcAttr configures cmd so the child runs in a fresh process group.
func SetSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
}

// Kill terminates the child process and any descendants by signalling the
// negated process group ID. force == true sends SIGKILL, otherwise SIGTERM.
func Kill(cmd *exec.Cmd, force bool) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	sig := syscall.SIGTERM
	if force {
		sig = syscall.SIGKILL
	}
	pgid, err := syscall.Getpgid(cmd.Process.Pid)
	if err != nil {
		// Fall back to killing the process directly.
		return cmd.Process.Signal(sig)
	}
	return syscall.Kill(-pgid, sig)
}
