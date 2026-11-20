package testenv

import (
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRedactContainerLogs(t *testing.T) {
	secrets := secretsFromEnv([]string{
		"RABBITMQ_DEFAULT_PASS=pass-value",
		"AUTH_TOKEN=token-value",
		"SESSION_AMQP_URL=amqp://user:percent%40pass@broker:5672/",
		"MYSQL_DSN=user:db-secret@tcp(mysql:3306)/test",
		"SERVICE_URL=https://example.test/?token=query-secret",
		"RABBITMQ_ERLANG_COOKIE=cookie-secret",
		"PUBLIC_VALUE=visible",
	})
	sort.Slice(secrets, func(i, j int) bool { return len(secrets[i]) > len(secrets[j]) })
	logs := "pass-value token-value percent@pass percent%40pass db-secret query-secret cookie-secret " +
		"amqp://user:unknown-secret@broker:5672/ visible"
	redacted := redactLogs(logs, secrets)
	for _, secret := range []string{
		"pass-value", "token-value", "percent@pass", "percent%40pass", "db-secret",
		"query-secret", "cookie-secret", "unknown-secret",
	} {
		require.NotContains(t, redacted, secret)
	}
	require.Contains(t, redacted, "visible")
	require.Equal(t, 8, strings.Count(redacted, "[REDACTED]"))
}
