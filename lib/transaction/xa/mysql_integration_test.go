package xa

import (
	"encoding/hex"
	"strings"
	"testing"
	"uuid"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestXIDValidation(t *testing.T) {
	txID := uuid.New()

	tests := []struct {
		name    string
		xid     XID
		wantErr bool
	}{
		{
			name: "valid auth branch",
			xid:  XID{TransactionID: txID, Branch: "auth"},
		},
		{
			name: "valid identity branch",
			xid:  XID{TransactionID: txID, Branch: "identity"},
		},
		{
			name:    "nil transaction ID",
			xid:     XID{TransactionID: uuid.Nil(), Branch: "auth"},
			wantErr: true,
		},
		{
			name:    "empty branch",
			xid:     XID{TransactionID: txID},
			wantErr: true,
		},
		{
			name:    "branch too long",
			xid:     XID{TransactionID: txID, Branch: strings.Repeat("a", 65)},
			wantErr: true,
		},
		{
			name:    "invalid branch characters",
			xid:     XID{TransactionID: txID, Branch: "auth'; DROP TABLE users;"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.xid.validate()

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
		})
	}
}

func TestXIDSQLLiteral(t *testing.T) {
	txID := uuid.New()
	xid := XID{TransactionID: txID, Branch: "identity"}

	want := "0x" + hex.EncodeToString([]byte(txID.String())) +
		",0x" + hex.EncodeToString([]byte("identity")) + ",2026"

	assert.Equal(t, want, xid.sqlLiteral())
}
