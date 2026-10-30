package xa

import (
	"encoding/hex"
	"errors"
	"fmt"
	"uuid"
)

const formatID = 67

type XID struct {
	TransactionID uuid.UUID
	Branch        string
}

func (x XID) validate() error {
	if x.TransactionID == uuid.Nil() {
		return errors.New("xa: transaction ID must not be nil")
	}

	if len(x.Branch) == 0 || len(x.Branch) > 64 {
		return errors.New("xa: branch length must be between 1 and 64 bytes")
	}

	for _, ch := range x.Branch {
		if !((ch >= 'a' && ch <= 'z') ||
			(ch >= 'A' && ch <= 'Z') ||
			(ch >= '0' && ch <= '9') ||
			ch == '-' || ch == '_') {
			return errors.New("xa: branch contains invalid characters")
		}
	}

	return nil
}

func (x XID) sqlLiteral() string {
	globalID := hex.EncodeToString([]byte(x.TransactionID.String()))
	branch := hex.EncodeToString([]byte(x.Branch))

	return fmt.Sprintf("0x%s,0x%s,%d", globalID, branch, formatID)
}
