package domain

import "uuid"

type ModuleGrantRepository interface {
	Store(*ModuleGrant) error
	FindByUserIDAndModuleKey(uuid.UUID, ModuleKey) (*ModuleGrant, error)
	FindAllByUserID(uuid.UUID) ([]*ModuleGrant, error)
	Delete(uuid.UUID, ModuleKey) error
}

type SystemRoleRepository interface {
	FindByUserID(uuid.UUID) (*SystemRoleAssignment, error)
	Store(*SystemRoleAssignment) error
	Delete(uuid.UUID) error
}

type InitialSuperuserRepository interface {
	Bootstrap(*SystemRoleAssignment) error
}
