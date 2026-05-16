//go:build windows

package stats

import (
	"syscall"
	"unsafe"
)

// We dynamically resolve the Win32 functions we need rather than
// importing golang.org/x/sys/windows so this file has zero new
// dependencies. Everything here is pure Go (no CGo).

var (
	kernel32                = syscall.NewLazyDLL("kernel32.dll")
	psapi                   = syscall.NewLazyDLL("psapi.dll")
	procOpenProcess         = kernel32.NewProc("OpenProcess")
	procCloseHandle         = kernel32.NewProc("CloseHandle")
	procGetProcessMemInfo   = psapi.NewProc("GetProcessMemoryInfo")
	procK32GetProcessMemory = kernel32.NewProc("K32GetProcessMemoryInfo")
	procGetProcessTimes     = kernel32.NewProc("GetProcessTimes")
)

const (
	processQueryLimitedInfo = 0x1000
	processQueryInfo        = 0x0400
	processVmRead           = 0x0010
)

// processMemoryCounters mirrors the Win32 PROCESS_MEMORY_COUNTERS struct.
// Field sizes match the platform pointer width (uintptr).
type processMemoryCounters struct {
	cb                         uint32
	pageFaultCount             uint32
	peakWorkingSetSize         uintptr
	workingSetSize             uintptr
	quotaPeakPagedPoolUsage    uintptr
	quotaPagedPoolUsage        uintptr
	quotaPeakNonPagedPoolUsage uintptr
	quotaNonPagedPoolUsage     uintptr
	pagefileUsage              uintptr
	peakPagefileUsage          uintptr
}

// memoryRSS returns the working-set size of pid in bytes, or 0 if it
// cannot be determined.
func memoryRSS(pid int) uint64 {
	if pid <= 0 {
		return 0
	}

	// Try the lighter-weight access right first; fall back to the older
	// one for compatibility with earlier Windows versions.
	h, _, _ := procOpenProcess.Call(uintptr(processQueryLimitedInfo), 0, uintptr(pid))
	if h == 0 {
		h, _, _ = procOpenProcess.Call(uintptr(processQueryInfo|processVmRead), 0, uintptr(pid))
		if h == 0 {
			return 0
		}
	}
	defer procCloseHandle.Call(h)

	var pmc processMemoryCounters
	pmc.cb = uint32(unsafe.Sizeof(pmc))

	// K32GetProcessMemoryInfo (kernel32) is preferred on modern Windows;
	// fall back to GetProcessMemoryInfo (psapi) for older builds.
	ret, _, _ := procK32GetProcessMemory.Call(h, uintptr(unsafe.Pointer(&pmc)), uintptr(pmc.cb))
	if ret == 0 {
		ret, _, _ = procGetProcessMemInfo.Call(h, uintptr(unsafe.Pointer(&pmc)), uintptr(pmc.cb))
		if ret == 0 {
			return 0
		}
	}
	return uint64(pmc.workingSetSize)
}

// fileTime is the Win32 FILETIME struct (100ns intervals since 1601-01-01).
type fileTime struct {
	LowDateTime  uint32
	HighDateTime uint32
}

func (ft fileTime) toNs() uint64 {
	t := uint64(ft.HighDateTime)<<32 | uint64(ft.LowDateTime)
	return t * 100 // FILETIME is in 100-nanosecond intervals
}

// cpuTimes returns the accumulated kernel and user CPU time in nanoseconds
// for the given pid. Returns (0, 0, false) if unavailable.
func cpuTimes(pid int) (kernelNs, userNs uint64, ok bool) {
	if pid <= 0 {
		return 0, 0, false
	}

	h, _, _ := procOpenProcess.Call(uintptr(processQueryLimitedInfo), 0, uintptr(pid))
	if h == 0 {
		h, _, _ = procOpenProcess.Call(uintptr(processQueryInfo), 0, uintptr(pid))
		if h == 0 {
			return 0, 0, false
		}
	}
	defer procCloseHandle.Call(h)

	var creationTime, exitTime, kernelTime, userTime fileTime
	ret, _, _ := procGetProcessTimes.Call(
		h,
		uintptr(unsafe.Pointer(&creationTime)),
		uintptr(unsafe.Pointer(&exitTime)),
		uintptr(unsafe.Pointer(&kernelTime)),
		uintptr(unsafe.Pointer(&userTime)),
	)
	if ret == 0 {
		return 0, 0, false
	}
	return kernelTime.toNs(), userTime.toNs(), true
}
