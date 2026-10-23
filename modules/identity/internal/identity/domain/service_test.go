package domain

import (
	"errors"
	"testing"
	"uuid"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeIdentityRepository struct {
	nextID       uuid.UUID
	nextIDErr    error
	existingUser *User
	findErr      error
	findByIDErr  error
	storeErr     error
	updateErr    error

	storedUser  *User
	updatedUser *User
	foundLogin  string
	foundID     uuid.UUID

	nextIDCalls   int
	findCalls     int
	storeCalls    int
	findByIDCalls int
	updateCalls   int
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

func (f *fakeIdentityRepository) FindByID(userID uuid.UUID) (*User, error) {
	f.findByIDCalls++
	f.foundID = userID
	return f.existingUser, f.findByIDErr
}

func (f *fakeIdentityRepository) Update(user *User) error {
	f.updateCalls++
	f.updatedUser = user
	return f.updateErr
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

	require.NoError(t, err)
	assert.Equal(t, expectedID, id)
	assert.Equal(t, 1, repo.findCalls, "FindByLogin calls")
	assert.Equal(t, 1, repo.nextIDCalls, "NextID calls")
	assert.Equal(t, 1, repo.storeCalls, "Store calls")
	assert.Equal(t, testAlexLogin, repo.foundLogin)
	require.NotNil(t, repo.storedUser)

	user := repo.storedUser
	assert.Equal(t, expectedID, user.id)
	assert.Equal(t, "Alex", user.firstName)
	assert.Equal(t, "Smith", user.lastName)
	assert.Equal(t, "James", user.middleName)
	assert.Equal(t, testAlexLogin, user.login)
	assert.Equal(t, Email("alex@example.com"), user.email)
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

			require.ErrorIs(t, err, tt.wantErr)

			assert.Equal(t, uuid.Nil(), id)

			assert.Equal(t, tt.wantFind, repo.findCalls)

			assert.Equal(t, tt.wantNextID, repo.nextIDCalls)

			assert.Equal(t, tt.wantStore, repo.storeCalls)
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

			require.ErrorIs(t, err, tt.wantErr)

			if tt.wantErr != nil {
				assert.Nil(t, user)
			} else {
				assert.Same(t, tt.foundUser, user)
			}

			assert.Equal(t, tt.wantCalls, repo.findCalls)

			assert.Equal(t, tt.wantLogin, repo.foundLogin)
		})
	}
}

func TestIdentityService_Update(t *testing.T) {
	const (
		updatedFirstName = "Ann"
		updatedLastName  = "Jones"
		updatedEmail     = "ann@example.com"
	)

	userID := uuid.New()
	current, err := NewUser(userID, "Alex", "Smith", "", testAlexLogin, Email("alex@example.com"))
	require.NoError(t, err)

	tests := []struct {
		name    string
		first   string
		last    string
		middle  string
		email   string
		found   *User
		wantErr error
	}{
		{name: "updates profile", first: updatedFirstName, last: updatedLastName, middle: "Lee", email: updatedEmail, found: current},
		{name: "missing user", first: updatedFirstName, last: updatedLastName, email: updatedEmail, wantErr: ErrUserNotFound},
		{name: "empty first name", first: "", last: updatedLastName, email: updatedEmail, found: current, wantErr: ErrInvalidFirstName},
		{name: "invalid email", first: updatedFirstName, last: updatedLastName, email: "invalid", found: current, wantErr: ErrInvalidEmail},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeIdentityRepository{existingUser: tt.found}
			_, err := NewIdentityService(repo).Update(userID, tt.first, tt.last, tt.middle, tt.email)
			require.ErrorIs(t, err, tt.wantErr)
			if tt.wantErr == nil {
				require.NotNil(t, repo.updatedUser)
				require.Equal(t, testAlexLogin, repo.updatedUser.login)
				require.Equal(t, tt.middle, repo.updatedUser.middleName)
			}
		})
	}
}
