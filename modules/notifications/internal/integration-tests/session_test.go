package integration_tests

import (
	"context"
	"net/http"
	"testing"
	"time"
	"uuid"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/wait"

	publicapi "notifications/api/server/publicapi"
)

func TestSessionUnavailableFailsClosed(t *testing.T) {
	_, token := newSession(t, uuid.New())
	client, err := publicapi.NewClientWithResponses("http://" + testEnv.conf.HTTP.Address)
	require.NoError(t, err)

	timeout := 5 * time.Second
	err = testEnv.sessionContainer.Stop(t.Context(), &timeout)
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		err := testEnv.sessionContainer.Start(ctx)
		if err != nil {
			t.Errorf("restart Session: %v", err)
			return
		}
		err = wait.ForHTTP("/healthz").WithPort("8082/tcp").
			WithStatusCodeMatcher(func(code int) bool { return code == http.StatusNoContent }).
			WaitUntilReady(ctx, testEnv.sessionContainer)
		if err != nil {
			t.Errorf("wait for Session: %v", err)
		}
	})

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	response, err := client.ListNotificationsWithResponse(ctx, nil,
		func(_ context.Context, request *http.Request) error {
			request.Header.Set("Cookie", "session="+token)
			return nil
		},
	)
	require.NoError(t, err)
	require.Equal(t, http.StatusServiceUnavailable, response.StatusCode())
}
