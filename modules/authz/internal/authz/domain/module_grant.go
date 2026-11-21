package domain

import (
	"errors"
	"time"
	"uuid"
)

var (
	ErrInvalidUserID            = errors.New("invalid user ID")
	ErrInvalidGrantedBy         = errors.New("invalid grant issuer")
	ErrInvalidGrantedAt         = errors.New("invalid grant time")
	ErrModuleGrantAlreadyExists = errors.New("module grant already exists")
)

type ModuleGrant struct {
	userID    uuid.UUID
	moduleKey ModuleKey
	grantedBy *uuid.UUID
	grantedAt time.Time
}

type ModuleGrantData struct {
	UserID    uuid.UUID
	ModuleKey ModuleKey
	GrantedBy *uuid.UUID
	GrantedAt time.Time
}

func NewModuleGrant(
	userID uuid.UUID,
	moduleKey ModuleKey,
	grantedBy *uuid.UUID,
	grantedAt time.Time,
) (*ModuleGrant, error) {
	if userID == uuid.Nil() {
		return nil, ErrInvalidUserID
	}
	validatedModuleKey, err := NewModuleKey(moduleKey.String())
	if err != nil {
		return nil, err
	}
	if grantedBy != nil && *grantedBy == uuid.Nil() {
		return nil, ErrInvalidGrantedBy
	}
	if grantedAt.IsZero() {
		return nil, ErrInvalidGrantedAt
	}

	return &ModuleGrant{
		userID:    userID,
		moduleKey: validatedModuleKey,
		grantedBy: cloneUUID(grantedBy),
		grantedAt: grantedAt.UTC(),
	}, nil
}

func (g *ModuleGrant) Data() ModuleGrantData {
	return ModuleGrantData{
		UserID:    g.userID,
		ModuleKey: g.moduleKey,
		GrantedBy: cloneUUID(g.grantedBy),
		GrantedAt: g.grantedAt,
	}
}

func cloneUUID(value *uuid.UUID) *uuid.UUID {
	if value == nil {
		return nil
	}
	uuidCopy := *value
	return &uuidCopy
}
