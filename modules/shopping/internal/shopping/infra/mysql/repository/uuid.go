package repository

import (
	"fmt"
	"uuid"
)

func parseBinaryUUID(raw []byte) (uuid.UUID, error) {
	if len(raw) != 16 {
		return uuid.Nil(), fmt.Errorf("invalid UUID length %d", len(raw))
	}
	var id uuid.UUID
	copy(id[:], raw)
	return id, nil
}
