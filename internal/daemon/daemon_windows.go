//go:build windows

// Package daemon contains platform-specific helpers for launching gopmd as
// a detached background process from the CLI.
package daemon

import (
	"os"
	"os/exec"
	"syscall"
)

const envMarker = "GOPM_DAEMON=1"

// Windows process creation flags. We use CREATE_NO_WINDOW to suppress the
// console window for the background daemon and CREATE_NEW_PROCESS_GROUP so
// signals sent to the parent shell don't propagate to gopmd. We
// deliberately do NOT use DETACHED_PROCESS — combining it with the
// default stdio handles tends to leave gopmd with broken file descriptors
// and an immediate silent exit on some Windows builds.
const createNoWindow = 0x08000000

// IsDaemonChild reports whether the current process was launched via Spawn.
func IsDaemonChild() bool {
	return os.Getenv("GOPM_DAEMON") == "1"
}

// Spawn starts the given binary as a background daemon. It returns once
// the child has been forked; the parent does not wait for the child.
func Spawn(binary string, args []string) error {
	cmd := exec.Command(binary, args...)
	cmd.Env = append(os.Environ(), envMarker)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | createNoWindow,
	}
	// Leaving Stdin/Stdout/Stderr nil lets exec.Start hand the child the
	// usual NUL-device handles on Windows, which is what we want.
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Start()
}
