package domain

import (
	"testing"
	"uuid"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewEmail(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    Email
		wantErr error
	}{
		{
			name:  "valid email",
			input: "alex@example.com",
			want:  Email("alex@example.com"),
		},
		{
			name:  "trim spaces",
			input: "  alex@example.com  ",
			want:  Email("alex@example.com"),
		},
		{
			name:    "empty email",
			input:   "",
			wantErr: ErrInvalidEmail,
		},
		{
			name:    "only spaces",
			input:   testWhitespace,
			wantErr: ErrInvalidEmail,
		},
		{
			name:    "invalid format",
			input:   "invalid-email",
			wantErr: ErrInvalidEmail,
		},
		{
			name:    "display name not allowed",
			input:   "Alex <alex@example.com>",
			wantErr: ErrInvalidEmail,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewEmail(tt.input)

			require.ErrorIs(t, err, tt.wantErr)

			if err != nil {
				assert.Empty(t, got)
				return
			}

			assert.Equal(t, tt.want, got)

			assert.Equal(t, string(tt.want), got.String())
		})
	}
}

func TestNewUserValidatesRequiredFields(t *testing.T) {
	tests := []struct {
		name      string
		id        uuid.UUID
		firstName string
		lastName  string
		login     string
		email     string
		wantErr   error
	}{
		{
			name:      "valid user",
			id:        uuid.New(),
			firstName: " Ada ",
			lastName:  " Lovelace ",
			login:     " ada ",
			email:     "ada@example.com",
		},
		{
			name:      "missing ID",
			firstName: testFirstName,
			lastName:  testLastName,
			login:     testLogin,
			email:     testEmail,
			wantErr:   ErrInvalidUserID,
		},
		{
			name:     "missing first name",
			id:       uuid.New(),
			lastName: testLastName,
			login:    testLogin,
			email:    testEmail,
			wantErr:  ErrInvalidFirstName,
		},
		{
			name:      "missing last name",
			id:        uuid.New(),
			firstName: testFirstName,
			login:     testLogin,
			email:     testEmail,
			wantErr:   ErrInvalidLastName,
		},
		{
			name:      "missing login",
			id:        uuid.New(),
			firstName: testFirstName,
			lastName:  testLastName,
			email:     testEmail,
			wantErr:   ErrInvalidLogin,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user, err := NewUser(
				tt.id,
				tt.firstName,
				tt.lastName,
				"",
				tt.login,
				Email(tt.email),
			)
			require.ErrorIs(t, err, tt.wantErr)
			if tt.wantErr == nil {
				require.NotNil(t, user)
				require.Equal(t, testFirstName, user.Data().FirstName)
				require.Equal(t, testLogin, user.Data().Login)
			}
		})
	}
}
