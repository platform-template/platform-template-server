//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"e2e/internal/testenv"
)

var env *testenv.Environment

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) (result int) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	var err error
	env, err = testenv.Start(ctx, "compose.yaml")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer func() {
		if err := env.Close(); err != nil {
			fmt.Fprintln(os.Stderr, "cleanup:", err)
			result = 1
		}
	}()
	return m.Run()
}
