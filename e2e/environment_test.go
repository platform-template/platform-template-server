//go:build e2e

package e2e

import (
	"context"
	"database/sql"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

func TestConnectionReuse(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()

	const workers = 12
	var wg sync.WaitGroup
	connections := make([]*grpc.ClientConn, workers)
	databases := make([]*sql.DB, workers)
	errs := make([]error, workers)
	for i := range workers {
		wg.Go(func() {
			connections[i], errs[i] = env.GRPC(ctx, "identity")
			if errs[i] == nil {
				databases[i], errs[i] = env.MySQL(ctx, "identity-mysql")
			}
		})
	}
	wg.Wait()
	for i := range workers {
		require.NoError(t, errs[i])
		require.Same(t, connections[0], connections[i])
		require.Same(t, databases[0], databases[i])
	}
}

func TestConnectionErrors(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	_, err := env.GRPC(ctx, "missing-service")
	require.ErrorContains(t, err, "missing-service")
	_, err = env.MySQL(ctx, "identity")
	require.ErrorContains(t, err, "MYSQL_DATABASE")
}
