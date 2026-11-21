package fixtures

import (
	publicapi "auth/api/server/publicapi"
	"crypto/rand"
	"encoding/hex"

	openapi "github.com/oapi-codegen/runtime/types"
)

func User() (publicapi.RegisterRequest, error) {
	var id [12]byte
	if _, err := rand.Read(id[:]); err != nil {
		return publicapi.RegisterRequest{}, err
	}
	suffix := hex.EncodeToString(id[:])
	middle := "E2E"
	return publicapi.RegisterRequest{
		FirstName: "Ada", LastName: "Lovelace", MiddleName: &middle,
		Login: "e2e-" + suffix, Email: openapi.Email("e2e-" + suffix + "@example.com"),
		Password: "test-password-1",
	}, nil
}
