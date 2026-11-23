package public

import (
	"testing"

	"github.com/stretchr/testify/require"

	publicapi "audit/api/server/publicapi"
)

func TestAuditSecurityScheme(t *testing.T) {
	spec, err := publicapi.GetSpec()
	require.NoError(t, err)
	require.NotNil(t, spec.Components)
	scheme := spec.Components.SecuritySchemes["sessionCookie"]
	require.NotNil(t, scheme)
	require.NotNil(t, scheme.Value)
	require.Equal(t, "apiKey", scheme.Value.Type)
	require.Equal(t, "cookie", scheme.Value.In)
	require.Equal(t, "session", scheme.Value.Name)

	for _, path := range []string{
		"/api/public/v1/audit/entries",
		"/api/public/v1/audit/entries/by-event-id",
	} {
		operation := spec.Paths.Value(path).Get
		require.NotNil(t, operation.Security)
		require.Len(t, *operation.Security, 1)
		require.Contains(t, (*operation.Security)[0], "sessionCookie")
	}
	require.Nil(t, spec.Paths.Value("/api/public/v1/echo").Post.Security)
}
