package healthcheck

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCheckerReportsReady(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	checker := New(srv.URL, time.Millisecond, time.Second, 1)
	defer checker.Stop()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := checker.Start(ctx)
	select {
	case got := <-ch:
		if got != StatusReady {
			t.Fatalf("status = %v, want %v", got, StatusReady)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for ready status")
	}
}

func TestCheckerReportsUnhealthy(t *testing.T) {
	checker := New("http://127.0.0.1:1", time.Millisecond, time.Millisecond, 1)
	defer checker.Stop()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := checker.Start(ctx)
	select {
	case got := <-ch:
		if got != StatusUnhealthy {
			t.Fatalf("status = %v, want %v", got, StatusUnhealthy)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for unhealthy status")
	}
}
