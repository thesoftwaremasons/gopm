//go:build linux || darwin

// Package daemon contains platform-specific helpers for launching gopmd as
// a detached background process from the CLI.
package daemon

import (
	"os"
	"os/exec"
	"syscall"
)

// envMarker is set in the child so gopmd knows it has been re-execed as the
// detached daemon (rather than run interactively by the user).
const envMarker = "GOPM_DAEMON=1"

// IsDaemonChild reports whether the current process was launched via Spawn.
func IsDaemonChild() bool {
	return os.Getenv("GOPM_DAEMON") == "1"
}

// Spawn starts the given binary as a detached daemon. It returns once the
// child has been forked; the parent does not wait for the child.
func Spawn(binary string, args []string) error {
	cmd := exec.Command(binary, args...)
	cmd.Env = append(os.Environ(), envMarker)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true,
	}
	// Detach standard streams.
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Start()
}
