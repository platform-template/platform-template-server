package domain

import (
	"errors"
	"fmt"
	"strings"
	"uuid"
)

var (
	ErrLoginAlreadyExists = errors.New("login already exists")
	ErrUserNotFound       = errors.New("user not found")
)

type IdentityService struct {
	repo IdentityRepository
}

func NewIdentityService(repo IdentityRepository) *IdentityService {
	return &IdentityService{
		repo: repo,
	}
}

func (s *IdentityService) Create(
	firstName string,
	lastName string,
	middleName *string,
	loginSnapshot string,
	email string,
) (uuid.UUID, error) {
	loginSnapshot = strings.TrimSpace(loginSnapshot)
	if loginSnapshot == "" {
		return uuid.Nil(), fmt.Errorf("create user: %w", ErrInvalidLogin)
	}
	id, err := s.repo.NextID()
	if err != nil {
		return uuid.Nil(), err
	}

	verifiedEmail, err := NewEmail(email)
	if err != nil {
		return uuid.Nil(), fmt.Errorf("create user: %w", err)
	}

	user, err := NewUser(
		id,
		firstName,
		lastName,
		middleName,
		loginSnapshot,
		verifiedEmail,
	)
	if err != nil {
		return uuid.Nil(), fmt.Errorf("create user: new user: %w", err)
	}
	err = s.repo.Store(user)
	if err != nil {
		return uuid.Nil(), fmt.Errorf("create user: store: %w", err)
	}

	return id, nil
}

func (s *IdentityService) FindByLoginSnapshot(loginSnapshot string) (*User, error) {
	loginSnapshot = strings.TrimSpace(loginSnapshot)
	if loginSnapshot == "" {
		return nil, fmt.Errorf("find user by login: %w", ErrInvalidLogin)
	}

	user, err := s.repo.FindByLoginSnapshot(loginSnapshot)
	if err != nil {
		return nil, fmt.Errorf("find user by login: %w", err)
	}
	if user == nil {
		return nil, fmt.Errorf("find user by login: %w", ErrUserNotFound)
	}

	return user, nil
}

func (s *IdentityService) FindByID(userID uuid.UUID) (*User, error) {
	if userID == uuid.Nil() {
		return nil, ErrInvalidUserID
	}

	user, err := s.repo.FindByID(userID)
	if err != nil {
		return nil, fmt.Errorf("find user by ID: %w", err)
	}
	if user == nil {
		return nil, fmt.Errorf("find user by ID: %w", ErrUserNotFound)
	}

	return user, nil
}

func (s *IdentityService) Update(
	userID uuid.UUID,
	firstName,
	lastName string,
	middleName *string,
	email string,
) (*User, error) {
	current, err := s.FindByID(userID)
	if err != nil {
		return nil, fmt.Errorf("update user: %w", err)
	}

	updated, err := NewUser(
		userID,
		firstName,
		lastName,
		middleName,
		current.Data().LoginSnapshot,
		Email(email),
	)
	if err != nil {
		return nil, fmt.Errorf("update user: %w", err)
	}
	if err := s.repo.Update(updated); err != nil {
		return nil, fmt.Errorf("update user: %w", err)
	}

	return updated, nil
}
