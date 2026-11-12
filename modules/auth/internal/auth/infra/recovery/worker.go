package recovery

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

type Recoverer interface {
	RecoverPending(ctx context.Context) error
}

type Config struct {
	Interval       time.Duration
	AttemptTimeout time.Duration
}

type Worker struct {
	recoverer Recoverer
	logger    *slog.Logger
	config    Config
}

func NewWorker(
	recoverer Recoverer,
	logger *slog.Logger,
	config Config,
) (*Worker, error) {
	if config.Interval <= 0 {
		return nil, fmt.Errorf("invalid recovery interval: %s", config.Interval)
	}

	if config.AttemptTimeout <= 0 {
		return nil, fmt.Errorf(
			"invalid recovery attempt timeout: %s",
			config.AttemptTimeout,
		)
	}

	return &Worker{
		recoverer: recoverer,
		logger:    logger,
		config:    config,
	}, nil
}

func (w *Worker) Run(ctx context.Context) error {
	ticker := time.NewTicker(w.config.Interval)
	defer ticker.Stop()

	for {
		if ctx.Err() != nil {
			return nil
		}

		attemptCtx, cancel := context.WithTimeout(
			ctx,
			w.config.AttemptTimeout,
		)

		err := w.recoverer.RecoverPending(attemptCtx)
		cancel()

		if err != nil && ctx.Err() == nil {
			w.logger.ErrorContext(
				ctx,
				"registration recovery failed",
				slog.Any("error", err),
			)
		}

		select {
		case <-ctx.Done():
			return nil

		case <-ticker.C:
		}
	}
}
