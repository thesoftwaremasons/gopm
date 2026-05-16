// Package healthcheck implements HTTP health polling for managed processes.
package healthcheck

import (
	"context"
	"net/http"
	"time"
)

// Status represents the health state reported by the checker.
type Status int

const (
	StatusUnknown   Status = iota
	StatusReady            // at least one recent success
	StatusUnhealthy        // consecutive failures >= retries threshold
)

// Checker polls a URL on a fixed interval and reports status changes on a
// channel. It is safe to create and discard multiple checkers.
type Checker struct {
	url      string
	interval time.Duration
	timeout  time.Duration
	retries  int
	statusCh chan Status
	stopCh   chan struct{}
}

// New creates a Checker. interval and timeout default to 5s and 2s
// respectively if zero. retries defaults to 3 if zero.
func New(url string, interval, timeout time.Duration, retries int) *Checker {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	if retries <= 0 {
		retries = 3
	}
	return &Checker{
		url:      url,
		interval: interval,
		timeout:  timeout,
		retries:  retries,
		statusCh: make(chan Status, 4),
		stopCh:   make(chan struct{}),
	}
}

// Start begins polling in a background goroutine. The returned channel
// receives Status values when the status changes. The channel is closed
// when polling stops. ctx cancellation also stops polling.
func (c *Checker) Start(ctx context.Context) <-chan Status {
	go c.run(ctx)
	return c.statusCh
}

// Stop signals the checker to stop polling.
func (c *Checker) Stop() {
	select {
	case <-c.stopCh:
	default:
		close(c.stopCh)
	}
}

func (c *Checker) run(ctx context.Context) {
	defer close(c.statusCh)

	client := &http.Client{Timeout: c.timeout}
	failures := 0
	current := StatusUnknown

	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.stopCh:
			return
		case <-ticker.C:
			ok := c.check(client)
			var next Status
			if ok {
				failures = 0
				next = StatusReady
			} else {
				failures++
				if failures >= c.retries {
					next = StatusUnhealthy
				} else {
					next = current // don't change until threshold
				}
			}
			if next != current {
				current = next
				select {
				case c.statusCh <- current:
				default:
				}
			}
		}
	}
}

func (c *Checker) check(client *http.Client) bool {
	resp, err := client.Get(c.url)
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 400
}
