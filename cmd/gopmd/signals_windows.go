//go:build windows

package main

import "os"

// shutdownSignals lists the OS signals that should trigger a graceful
// daemon shutdown on Windows. Windows only delivers os.Interrupt
// (Ctrl+C / Ctrl+Break) reliably to a Go process — there is no SIGTERM.
func shutdownSignals() []os.Signal {
	return []os.Signal{os.Interrupt}
}
