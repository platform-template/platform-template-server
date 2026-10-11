package domain

import (
	"errors"
	"testing"
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
			input:   "   ",
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
