//go:build linux || darwin

package main

import (
	"os"
	"syscall"
)

// shutdownSignals lists the OS signals that should trigger a graceful
// daemon shutdown on Unix.
func shutdownSignals() []os.Signal {
	return []os.Signal{syscall.SIGINT, syscall.SIGTERM}
}
