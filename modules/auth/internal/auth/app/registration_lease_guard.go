package app

import (
	"context"
	"errors"
	"fmt"
	"time"
)

const (
	registrationLeaseTTL           = 60 * time.Second
	registrationLeaseRenewInterval = 20 * time.Second
	registrationLeaseOperationTTL  = 5 * time.Second
)

func (c *RegistrationCoordinator) withLease(
	ctx context.Context,
	lease RegistrationLease,
	work func(context.Context) error,
) error {
	workCtx, cancelWork := context.WithCancel(ctx)
	defer cancelWork()

	stopRenew := make(chan struct{})
	renewDone := make(chan error, 1)

	go func() {
		ticker := time.NewTicker(registrationLeaseRenewInterval)
		defer ticker.Stop()

		for {
			select {
			case <-stopRenew:
				renewDone <- nil
				return

			case <-ticker.C:
				renewCtx, cancel := context.WithTimeout(
					context.Background(),
					registrationLeaseOperationTTL,
				)

				renewed, err := c.leases.Renew(
					renewCtx,
					lease,
					registrationLeaseTTL,
				)
				cancel()

				if err != nil {
					cancelWork()
					renewDone <- fmt.Errorf(
						"%w: renew lease: %w",
						ErrRegistrationLeaseLost,
						err,
					)
					return
				}

				if !renewed {
					cancelWork()
					renewDone <- ErrRegistrationLeaseLost
					return
				}
			}
		}
	}()

	workErr := work(workCtx)

	close(stopRenew)
	renewErr := <-renewDone

	releaseCtx, cancel := context.WithTimeout(
		context.Background(),
		registrationLeaseOperationTTL,
	)
	_, _ = c.leases.Release(releaseCtx, lease)
	cancel()

	return errors.Join(workErr, renewErr)
}
