package domain

import (
	"fmt"
	"strings"
	"uuid"
)

type AuthService struct {
	repo   AuthRepository
	hasher PasswordHasher
}

func NewAuthService(repo AuthRepository, hasher PasswordHasher) *AuthService {
	return &AuthService{
		repo:   repo,
		hasher: hasher,
	}
}

func (s *AuthService) Create(userID uuid.UUID, login, password string) error {
	if userID == uuid.Nil() {
		return fmt.Errorf("create auth: %w", ErrInvalidUserID)
	}

	err := ValidatePassword(password)
	if err != nil {
		return fmt.Errorf("create auth: validate password: %w", err)
	}

	auth, err := s.repo.FindByUserID(userID)
	if err != nil {
		return fmt.Errorf("create auth: find by user ID: %w", err)
	}
	if auth != nil {
		return fmt.Errorf("create auth: find by user ID: %w", ErrCredentialAlreadyExists)
	}

	passwordHash, err := s.hasher.Hash(password)
	if err != nil {
		return fmt.Errorf("create auth: hasher: %w", err)
	}

	auth, err = NewPasswordCredential(userID, login, passwordHash)
	if err != nil {
		return fmt.Errorf("create auth: creating credential: %w", err)
	}

	err = s.repo.Store(auth)
	if err != nil {
		return fmt.Errorf("create auth: store: %w", err)
	}

	return nil
}

func (s *AuthService) Check(login, password string) (uuid.UUID, bool, error) {
	login = strings.TrimSpace(login)
	if login == "" {
		return uuid.Nil(), false, fmt.Errorf("check auth: %w", ErrInvalidLogin)
	}

	credential, err := s.repo.FindByLogin(login)
	if err != nil {
		return uuid.Nil(), false, fmt.Errorf("check auth: find by login: %w", err)
	}
	if credential == nil {
		return uuid.Nil(), false, fmt.Errorf("check auth: %w", ErrCredentialNotFound)
	}

	match, err := s.hasher.Verify(password, credential.passwordHash)
	if err != nil {
		return uuid.Nil(), false, fmt.Errorf("check auth: verify password: %w", err)
	}

	return credential.userID, match, nil
}

func (s *AuthService) CheckByUserID(userID uuid.UUID, password string) (bool, error) {
	if userID == uuid.Nil() {
		return false, fmt.Errorf("check auth: %w", ErrInvalidUserID)
	}

	credential, err := s.repo.FindByUserID(userID)
	if err != nil {
		return false, fmt.Errorf("check auth: find by user ID: %w", err)
	}
	if credential == nil {
		return false, fmt.Errorf("check auth: %w", ErrCredentialNotFound)
	}

	match, err := s.hasher.Verify(password, credential.passwordHash)
	if err != nil {
		return false, fmt.Errorf("check auth: verify password: %w", err)
	}

	return match, nil
}

func (s *AuthService) ChangePassword(userID uuid.UUID, password string) error {
	if userID == uuid.Nil() {
		return fmt.Errorf("change password: %w", ErrInvalidUserID)
	}

	if err := ValidatePassword(password); err != nil {
		return fmt.Errorf("change password: %w", err)
	}

	credential, err := s.repo.FindByUserID(userID)
	if err != nil {
		return fmt.Errorf("change password: find by user ID: %w", err)
	}
	if credential == nil {
		return fmt.Errorf("change password: %w", ErrCredentialNotFound)
	}

	same, err := s.hasher.Verify(password, credential.passwordHash)
	if err != nil {
		return fmt.Errorf("change password: verify password: %w", err)
	}
	if same {
		return fmt.Errorf("change password: %w", ErrPasswordUnchanged)
	}

	newHash, err := s.hasher.Hash(password)
	if err != nil {
		return fmt.Errorf("change password: hash password: %w", err)
	}

	if err := credential.changePasswordHash(newHash); err != nil {
		return fmt.Errorf("change password: update credential: %w", err)
	}

	if err := s.repo.Change(credential); err != nil {
		return fmt.Errorf("change password: store credential: %w", err)
	}

	return nil
}
