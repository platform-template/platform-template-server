package domain

import (
	"errors"
	"uuid"
)

var (
	ErrInvalidSystemRole     = errors.New("invalid system role")
	ErrDefaultRoleAssignment = errors.New(
		"default user role must not be stored",
	)
	ErrInitialSuperuserExists = errors.New(
		"initial superuser already exists",
	)
)

type SystemRole string

const (
	SystemRoleUser      SystemRole = "user"
	SystemRoleSuperuser SystemRole = "superuser"
)

func NewSystemRole(raw string) (SystemRole, error) {
	role := SystemRole(raw)
	switch role {
	case SystemRoleUser, SystemRoleSuperuser:
		return role, nil
	default:
		return "", ErrInvalidSystemRole
	}
}

func (r SystemRole) HasGlobalPrivileges() bool {
	return r == SystemRoleSuperuser
}

type SystemRoleAssignment struct {
	userID uuid.UUID
	role   SystemRole
}

type SystemRoleAssignmentData struct {
	UserID uuid.UUID
	Role   SystemRole
}

func NewSystemRoleAssignment(
	userID uuid.UUID,
	role SystemRole,
) (*SystemRoleAssignment, error) {
	if userID == uuid.Nil() {
		return nil, ErrInvalidUserID
	}
	validatedRole, err := NewSystemRole(string(role))
	if err != nil {
		return nil, err
	}
	if validatedRole == SystemRoleUser {
		return nil, ErrDefaultRoleAssignment
	}
	return &SystemRoleAssignment{userID: userID, role: validatedRole}, nil
}

func (a *SystemRoleAssignment) Data() SystemRoleAssignmentData {
	return SystemRoleAssignmentData{UserID: a.userID, Role: a.role}
}
