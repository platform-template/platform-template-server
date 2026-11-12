package recovery

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"
)

type recovererFunc func(context.Context) error

func (f recovererFunc) RecoverPending(ctx context.Context) error {
	return f(ctx)
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestWorkerRetriesAfterFailure(t *testing.T) {
	ctx, cancel := context.WithTimeout(
		t.Context(),
		time.Second,
	)
	defer cancel()

	calls := 0
	failure := errors.New("identity unavailable")

	recoverer := recovererFunc(func(context.Context) error {
		calls++

		if calls == 1 {
			return failure
		}

		cancel()
		return nil
	})

	worker, err := NewWorker(
		recoverer,
		testLogger(),
		Config{
			Interval:       10 * time.Millisecond,
			AttemptTimeout: time.Second,
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	if err := worker.Run(ctx); err != nil {
		t.Fatalf("worker stopped with error: %v", err)
	}

	if calls != 2 {
		t.Errorf("recovery calls: got %d, want 2", calls)
	}
}

func TestWorkerRunsImmediately(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	calls := 0

	recoverer := recovererFunc(func(context.Context) error {
		calls++
		cancel()
		return nil
	})

	worker, err := NewWorker(
		recoverer,
		testLogger(),
		Config{
			Interval:       time.Hour,
			AttemptTimeout: time.Second,
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	if err := worker.Run(ctx); err != nil {
		t.Fatal(err)
	}

	if calls != 1 {
		t.Errorf("recovery calls: got %d, want 1", calls)
	}
}
