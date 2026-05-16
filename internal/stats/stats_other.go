//go:build !linux && !windows

package stats

// memoryRSS is unsupported on this platform today; return 0 so the list
// renderer can fall back to "-".
func memoryRSS(pid int) uint64 {
	return 0
}

// cpuTimes is unsupported on this platform; returns (0, 0, false).
func cpuTimes(pid int) (uint64, uint64, bool) {
	return 0, 0, false
}
