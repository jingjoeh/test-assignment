package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"
)

// These tests are deterministic (D16): no time.Sleep and no real ticker. The
// test owns an unbuffered tick channel, so each send returns only once the
// worker has taken that tick, and the test decides exactly how many ticks
// happen. After the last tick it stops the worker and waits for RunUserCount
// to return; from then on every log write is finished and can be asserted.

// counterFunc adapts a function to UserCounter.
type counterFunc func(ctx context.Context) (int64, error)

func (f counterFunc) Count(ctx context.Context) (int64, error) { return f(ctx) }

// sequence returns a counter that answers with results in order.
func sequence(results ...result) UserCounter {
	i := 0
	return counterFunc(func(context.Context) (int64, error) {
		r := results[i]
		i++
		return r.n, r.err
	})
}

type result struct {
	n   int64
	err error
}

type run struct {
	ticks  chan time.Time
	cancel context.CancelFunc
	done   chan struct{}
	logs   *bytes.Buffer
}

func start(t *testing.T, counter UserCounter) run {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	r := run{ticks: make(chan time.Time), cancel: cancel, done: make(chan struct{}), logs: &bytes.Buffer{}}
	logger := slog.New(slog.NewJSONHandler(r.logs, nil))
	go func() {
		defer close(r.done)
		RunUserCount(ctx, r.ticks, counter, logger)
	}()
	t.Cleanup(cancel)
	return r
}

func (r run) tick() { r.ticks <- time.Time{} }

// wait blocks until RunUserCount has returned. The timeout is only a
// failsafe so a broken worker fails the test instead of hanging it; it never
// drives the test.
func (r run) wait(t *testing.T) {
	t.Helper()
	select {
	case <-r.done:
	case <-time.After(5 * time.Second):
		t.Fatal("RunUserCount did not return")
	}
}

func (r run) entries(t *testing.T) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(r.logs.String()), "\n") {
		if line == "" {
			continue
		}
		var e map[string]any
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("log line is not JSON: %v; line = %s", err, line)
		}
		out = append(out, e)
	}
	return out
}

func TestRunUserCountLogsOnEveryTick(t *testing.T) {
	r := start(t, sequence(result{n: 0}, result{n: 3}, result{n: 7}))

	r.tick()
	r.tick()
	r.tick()
	r.cancel()
	r.wait(t)

	entries := r.entries(t)
	want := []float64{0, 3, 7}
	if len(entries) != len(want) {
		t.Fatalf("got %d log entries, want %d:\n%s", len(entries), len(want), r.logs)
	}
	for i, e := range entries {
		if e["msg"] != "user count" || e["level"] != "INFO" || e["total_users"] != want[i] {
			t.Errorf("entry %d = %v, want INFO \"user count\" with total_users=%v", i, e, want[i])
		}
	}
}

func TestRunUserCountKeepsRunningAfterError(t *testing.T) {
	r := start(t, sequence(result{n: 2}, result{err: errors.New("db down")}, result{n: 5}))

	r.tick()
	r.tick()
	r.tick()
	r.cancel()
	r.wait(t)

	entries := r.entries(t)
	if len(entries) != 3 {
		t.Fatalf("got %d log entries, want 3:\n%s", len(entries), r.logs)
	}
	if entries[0]["total_users"] != float64(2) {
		t.Errorf("entry 0 = %v, want total_users=2", entries[0])
	}
	if entries[1]["level"] != "ERROR" || entries[1]["msg"] != "user count failed" || entries[1]["error"] != "db down" {
		t.Errorf("entry 1 = %v, want an ERROR with the cause", entries[1])
	}
	if entries[2]["total_users"] != float64(5) {
		t.Errorf("entry 2 = %v, want total_users=5: the tick after an error must still count", entries[2])
	}
}

func TestRunUserCountStopsOnCancel(t *testing.T) {
	calls := 0
	r := start(t, counterFunc(func(context.Context) (int64, error) { calls++; return 0, nil }))

	r.cancel()
	r.wait(t)

	if calls != 0 || r.logs.Len() != 0 {
		t.Errorf("worker counted %d times and logged %q before any tick, want nothing", calls, r.logs)
	}
}

func TestRunUserCountStopsWhenTicksClosed(t *testing.T) {
	r := start(t, sequence(result{n: 1}))

	r.tick()
	close(r.ticks)
	r.wait(t) // returns without cancelling the context

	if entries := r.entries(t); len(entries) != 1 || entries[0]["total_users"] != float64(1) {
		t.Errorf("entries = %v, want one entry with total_users=1", entries)
	}
}

func TestRunUserCountPassesContextToCounter(t *testing.T) {
	type key struct{}
	var got any
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), key{}, "marker"))
	ticks := make(chan time.Time)
	done := make(chan struct{})
	go func() {
		defer close(done)
		RunUserCount(ctx, ticks, counterFunc(func(c context.Context) (int64, error) {
			got = c.Value(key{})
			return 0, nil
		}), slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)))
	}()

	ticks <- time.Time{}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("RunUserCount did not return")
	}
	if got != "marker" {
		t.Errorf("counter got context value %v, want the worker's context", got)
	}
}
