package domain

import (
	"errors"
	"testing"
	"uuid"
)

type fakeIdentityRepository struct {
	nextID       uuid.UUID
	nextIDErr    error
	existingUser *User
	findErr      error
	storeErr     error

	storedUser *User
	foundLogin string

	nextIDCalls int
	findCalls   int
	storeCalls  int
}

func (f *fakeIdentityRepository) NextID() (uuid.UUID, error) {
	f.nextIDCalls++
	return f.nextID, f.nextIDErr
}

func (f *fakeIdentityRepository) Store(user *User) error {
	f.storeCalls++
	f.storedUser = user
	return f.storeErr
}

func (f *fakeIdentityRepository) FindByLogin(login string) (*User, error) {
	f.findCalls++
	f.foundLogin = login
	return f.existingUser, f.findErr
}

var _ IdentityRepository = (*fakeIdentityRepository)(nil)

func TestIdentityService_Create(t *testing.T) {
	expectedID := uuid.New()

	repo := &fakeIdentityRepository{
		nextID: expectedID,
	}
	service := NewIdentityService(repo)

	id, err := service.Create(
		"  Alex  ",
		"  Smith  ",
		"  James  ",
		"  alex  ",
		"  alex@example.com  ",
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if id != expectedID {
		t.Errorf("expected ID %v, got %v", expectedID, id)
	}

	if repo.findCalls != 1 || repo.nextIDCalls != 1 || repo.storeCalls != 1 {
		t.Errorf("unexpected repository calls: find=%d, nextID=%d, store=%d",
			repo.findCalls, repo.nextIDCalls, repo.storeCalls)
	}

	if repo.foundLogin != testAlexLogin {
		t.Errorf("expected lookup login %q, got %q", testAlexLogin, repo.foundLogin)
	}

	if repo.storedUser == nil {
		t.Fatal("expected user to be stored")
	}

	user := repo.storedUser

	if user.id != expectedID {
		t.Errorf("expected stored ID %v, got %v", expectedID, user.id)
	}

	if user.firstName != "Alex" || user.lastName != "Smith" || user.middleName != "James" {
		t.Errorf("unexpected stored name: %q %q %q", user.firstName, user.lastName, user.middleName)
	}

	if user.login != testAlexLogin {
		t.Errorf("expected stored login %q, got %q", testAlexLogin, user.login)
	}

	if user.email != Email("alex@example.com") {
		t.Errorf("expected stored email %q, got %q", "alex@example.com", user.email)
	}
}

type createInput struct {
	firstName  string
	lastName   string
	middleName string
	login      string
	email      string
}

func validCreateInput() createInput {
	return createInput{
		firstName:  "Alex",
		lastName:   "Smith",
		middleName: "",
		login:      testAlexLogin,
		email:      "alex@example.com",
	}
}

func TestIdentityService_Create_Errors(t *testing.T) {
	findErr := errors.New("find failed")
	nextIDErr := errors.New("next ID failed")
	storeErr := errors.New("store failed")

	tests := []struct {
		name        string
		modifyInput func(*createInput)
		modifyRepo  func(*fakeIdentityRepository)
		wantErr     error
		wantFind    int
		wantNextID  int
		wantStore   int
	}{
		{
			name: "empty login",
			modifyInput: func(i *createInput) {
				i.login = ""
			},
			wantErr: ErrInvalidLogin,
		},
		{
			name: "whitespace login",
			modifyInput: func(i *createInput) {
				i.login = testWhitespace
			},
			wantErr: ErrInvalidLogin,
		},
		{
			name: "login already exists",
			modifyRepo: func(r *fakeIdentityRepository) {
				r.existingUser = &User{}
			},
			wantErr:  ErrLoginAlreadyExists,
			wantFind: 1,
		},
		{
			name: "find error",
			modifyRepo: func(r *fakeIdentityRepository) {
				r.findErr = findErr
			},
			wantErr:  findErr,
			wantFind: 1,
		},
		{
			name: "next ID error",
			modifyRepo: func(r *fakeIdentityRepository) {
				r.nextIDErr = nextIDErr
			},
			wantErr:    nextIDErr,
			wantFind:   1,
			wantNextID: 1,
		},
		{
			name: "invalid email",
			modifyInput: func(i *createInput) {
				i.email = "invalid-email"
			},
			wantErr:    ErrInvalidEmail,
			wantFind:   1,
			wantNextID: 1,
		},
		{
			name: "empty first name",
			modifyInput: func(i *createInput) {
				i.firstName = ""
			},
			wantErr:    ErrInvalidFirstName,
			wantFind:   1,
			wantNextID: 1,
		},
		{
			name: "empty last name",
			modifyInput: func(i *createInput) {
				i.lastName = testWhitespace
			},
			wantErr:    ErrInvalidLastName,
			wantFind:   1,
			wantNextID: 1,
		},
		{
			name: "store error",
			modifyRepo: func(r *fakeIdentityRepository) {
				r.storeErr = storeErr
			},
			wantErr:    storeErr,
			wantFind:   1,
			wantNextID: 1,
			wantStore:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := validCreateInput()

			repo := &fakeIdentityRepository{
				nextID: uuid.New(),
			}

			if tt.modifyInput != nil {
				tt.modifyInput(&input)
			}

			if tt.modifyRepo != nil {
				tt.modifyRepo(repo)
			}

			service := NewIdentityService(repo)

			id, err := service.Create(
				input.firstName,
				input.lastName,
				input.middleName,
				input.login,
				input.email,
			)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected error %v, got %v", tt.wantErr, err)
			}

			if id != uuid.Nil() {
				t.Errorf("expected nil UUID on error, got %v", id)
			}

			if repo.findCalls != tt.wantFind {
				t.Errorf("expected %d Find calls, got %d", tt.wantFind, repo.findCalls)
			}

			if repo.nextIDCalls != tt.wantNextID {
				t.Errorf("expected %d NextID calls, got %d", tt.wantNextID, repo.nextIDCalls)
			}

			if repo.storeCalls != tt.wantStore {
				t.Errorf("expected %d Store calls, got %d", tt.wantStore, repo.storeCalls)
			}
		})
	}
}

func TestIdentityService_FindByLogin(t *testing.T) {
	existingUser := &User{login: testAlexLogin}
	findErr := errors.New("find failed")

	tests := []struct {
		name      string
		login     string
		foundUser *User
		repoErr   error
		wantErr   error
		wantLogin string
		wantCalls int
	}{
		{
			name:      "user found",
			login:     "  alex  ",
			foundUser: existingUser,
			wantLogin: testAlexLogin,
			wantCalls: 1,
		},
		{
			name:    "empty login",
			login:   "",
			wantErr: ErrInvalidLogin,
		},
		{
			name:    "whitespace login",
			login:   testWhitespace,
			wantErr: ErrInvalidLogin,
		},
		{
			name:      "user not found",
			login:     "unknown",
			wantErr:   ErrUserNotFound,
			wantLogin: "unknown",
			wantCalls: 1,
		},
		{
			name:      "repository error",
			login:     testAlexLogin,
			repoErr:   findErr,
			wantErr:   findErr,
			wantLogin: testAlexLogin,
			wantCalls: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeIdentityRepository{
				existingUser: tt.foundUser,
				findErr:      tt.repoErr,
			}
			service := NewIdentityService(repo)

			user, err := service.FindByLogin(tt.login)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected error %v, got %v", tt.wantErr, err)
			}

			if tt.wantErr != nil {
				if user != nil {
					t.Error("expected nil user on error")
				}
			} else if user != tt.foundUser {
				t.Errorf("expected user %p, got %p", tt.foundUser, user)
			}

			if repo.findCalls != tt.wantCalls {
				t.Errorf("expected %d Find calls, got %d", tt.wantCalls, repo.findCalls)
			}

			if repo.foundLogin != tt.wantLogin {
				t.Errorf("expected lookup login %q, got %q", tt.wantLogin, repo.foundLogin)
			}
		})
	}
}
