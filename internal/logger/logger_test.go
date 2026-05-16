package logger

import (
	"strings"
	"testing"
	"time"
)

func TestTailKeepsRecentLines(t *testing.T) {
	l := New(3)
	l.Write("api", "one")
	l.Write("api", "two")
	l.Write("api", "three")
	l.Write("api", "four")

	got := l.Tail("api", 10)
	want := []string{"two", "three", "four"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("Tail() = %#v, want %#v", got, want)
	}
}

func TestTailFiltered(t *testing.T) {
	l := New(10)
	l.Write("api", "ready")
	l.Write("api", "error: failed")

	got := l.TailFiltered("api", 10, time.Now().Add(-time.Minute), "error")
	if len(got) != 1 || got[0] != "error: failed" {
		t.Fatalf("TailFiltered() = %#v", got)
	}
}

func TestSubscribeReceivesEvents(t *testing.T) {
	l := New(10)
	ch := l.Subscribe("api")
	defer l.Unsubscribe("api", ch)

	l.Write("api", "hello")

	select {
	case ev := <-ch:
		if ev.Name != "api" || ev.Line != "hello" {
			t.Fatalf("event = %#v", ev)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for log event")
	}
}
