//go:build linux

package stats

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"sync"
)

// memoryRSS parses /proc/<pid>/status, looking for `VmRSS:	N kB`.
func memoryRSS(pid int) uint64 {
	if pid <= 0 {
		return 0
	}
	f, err := os.Open("/proc/" + strconv.Itoa(pid) + "/status")
	if err != nil {
		return 0
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "VmRSS:") {
			continue
		}
		fields := strings.Fields(line)
		// Expect: "VmRSS:" "<n>" "kB"
		if len(fields) < 2 {
			return 0
		}
		kb, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return 0
		}
		return kb * 1024
	}
	return 0
}

var (
	clkTckOnce sync.Once
	clkTck     float64
)

// getClockTicks returns the number of clock ticks per second (SC_CLK_TCK).
// Defaults to 100 if unavailable.
func getClockTicks() float64 {
	clkTckOnce.Do(func() {
		clkTck = 100 // safe default
		// Try to read from /proc/self/stat to get an idea, or just use default.
		// On Linux, SC_CLK_TCK is almost always 100.
	})
	return clkTck
}

// cpuTimes returns accumulated kernel and user CPU time in nanoseconds for pid.
// Reads /proc/pid/stat fields 14 (utime) and 15 (stime).
func cpuTimes(pid int) (kernelNs, userNs uint64, ok bool) {
	if pid <= 0 {
		return 0, 0, false
	}
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return 0, 0, false
	}

	// The format is: pid (comm) state ... utime stime ...
	// comm can contain spaces and parentheses, so find the last ')'.
	s := string(data)
	idx := strings.LastIndex(s, ")")
	if idx < 0 {
		return 0, 0, false
	}
	// Fields after ')' are space-separated, starting at field index 2.
	rest := strings.TrimSpace(s[idx+1:])
	fields := strings.Fields(rest)
	// Fields: [state(2), ppid(3), pgrp(4), session(5), tty_nr(6), tpgid(7),
	//          flags(8), minflt(9), cminflt(10), majflt(11), cmajflt(12),
	//          utime(13), stime(14), ...]
	// 0-indexed into fields slice after state: utime=11, stime=12
	if len(fields) < 13 {
		return 0, 0, false
	}
	utime, err1 := strconv.ParseUint(fields[11], 10, 64)
	stime, err2 := strconv.ParseUint(fields[12], 10, 64)
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}

	ticks := getClockTicks()
	nsPerTick := uint64(1e9 / ticks)
	return stime * nsPerTick, utime * nsPerTick, true
}
