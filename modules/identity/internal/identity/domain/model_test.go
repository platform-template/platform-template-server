package domain

import (
	"errors"
	"testing"
	"uuid"
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

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected error %v, got %v", tt.wantErr, err)
			}

			if err != nil {
				if got != "" {
					t.Errorf("expected empty email on error, got %q", got)
				}
				return
			}

			if got != tt.want {
				t.Errorf("expected %q, got %q", tt.want, got)
			}

			if got.String() != string(tt.want) {
				t.Errorf("unexpected String() result: %q", got.String())
			}
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
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected %v, got %v", tt.wantErr, err)
			}
			if tt.wantErr == nil && (user.Data().FirstName != testFirstName || user.Data().Login != testLogin) {
				t.Fatal("user fields were not normalized")
			}
		})
	}
}
