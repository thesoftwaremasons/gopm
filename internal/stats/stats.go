// Package stats exposes per-process resource statistics (memory RSS and CPU%).
package stats

import (
	"context"
	"sync"
	"time"
)

// MemoryRSS returns the resident set size of the given pid in bytes,
// or 0 if it cannot be determined (unsupported platform, dead pid, etc.).
func MemoryRSS(pid int) uint64 {
	return memoryRSS(pid)
}

// cpuSample holds a pair of CPU time samples for delta calculation.
type cpuSample struct {
	kernelNs uint64
	userNs   uint64
	wallNs   int64
}

var (
	cpuMu      sync.Mutex
	cpuSamples = map[int]cpuSample{}
	cpuPercent = map[int]float64{}
	tracked    = map[int]bool{}
)

// TrackPID registers a pid for CPU sampling.
func TrackPID(pid int) {
	cpuMu.Lock()
	tracked[pid] = true
	cpuMu.Unlock()
}

// UntrackPID removes a pid from CPU sampling.
func UntrackPID(pid int) {
	cpuMu.Lock()
	delete(tracked, pid)
	delete(cpuSamples, pid)
	delete(cpuPercent, pid)
	cpuMu.Unlock()
}

// CPUPercent returns the latest CPU% for pid, or 0 if unavailable.
func CPUPercent(pid int) float64 {
	cpuMu.Lock()
	defer cpuMu.Unlock()
	return cpuPercent[pid]
}

// StartCPUSampler runs a background goroutine that samples all registered PIDs
// every 500ms and computes CPU%.
func StartCPUSampler(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				sampleAll()
			}
		}
	}()
}

func sampleAll() {
	cpuMu.Lock()
	pids := make([]int, 0, len(tracked))
	for pid := range tracked {
		pids = append(pids, pid)
	}
	cpuMu.Unlock()

	now := time.Now().UnixNano()
	for _, pid := range pids {
		k, u, ok := cpuTimes(pid)
		if !ok {
			continue
		}
		cpuMu.Lock()
		prev, hasPrev := cpuSamples[pid]
		cpuSamples[pid] = cpuSample{kernelNs: k, userNs: u, wallNs: now}
		if hasPrev && now > prev.wallNs {
			// Guard against PID recycling or any case where the new sample
			// is smaller than the previous one — uint64 subtraction would
			// otherwise wrap to ~10^19 and pin CPU% at insane values.
			if k >= prev.kernelNs && u >= prev.userNs {
				wallDelta := float64(now - prev.wallNs)
				cpuDelta := float64((k - prev.kernelNs) + (u - prev.userNs))
				if wallDelta > 0 {
					cpuPercent[pid] = (cpuDelta / wallDelta) * 100.0
				}
			} else {
				// Treat as a fresh process; drop stale percentage.
				cpuPercent[pid] = 0
			}
		}
		cpuMu.Unlock()
	}
}
