package domain

import (
	"errors"
	"testing"
	"uuid"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeIdentityRepository struct {
	existingUser *User
	findErr      error
	findByIDErr  error
	storeErr     error
	updateErr    error

	storedUser  *User
	updatedUser *User
	foundLogin  string
	foundID     uuid.UUID

	findCalls     int
	storeCalls    int
	findByIDCalls int
	updateCalls   int
}

func (f *fakeIdentityRepository) Store(user *User) error {
	f.storeCalls++
	f.storedUser = user
	return f.storeErr
}

func (f *fakeIdentityRepository) FindByLoginSnapshot(login string) (*User, error) {
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

func TestIdentityService_CreateWithID(t *testing.T) {
	userID := uuid.New()
	repo := &fakeIdentityRepository{}
	service := NewIdentityService(repo)

	err := service.CreateWithID(
		userID,
		"  Alex  ",
		"  Smith  ",
		stringPointer("  James  "),
		"  alex  ",
		"  alex@example.com  ",
	)

	require.NoError(t, err)
	assert.Zero(t, repo.findCalls, "identity must not check login uniqueness")
	assert.Equal(t, 1, repo.storeCalls)
	require.NotNil(t, repo.storedUser)

	user := repo.storedUser
	assert.Equal(t, userID, user.id)
	assert.Equal(t, "Alex", user.firstName)
	assert.Equal(t, "Smith", user.lastName)
	require.NotNil(t, user.middleName)
	assert.Equal(t, "James", *user.middleName)
	assert.Equal(t, testAlexLogin, user.loginSnapshot)
	assert.Equal(t, Email("alex@example.com"), user.email)
}

type createInput struct {
	firstName  string
	lastName   string
	middleName *string
	login      string
	email      string
}

func validCreateInput() createInput {
	return createInput{
		firstName:  "Alex",
		lastName:   "Smith",
		middleName: nil,
		login:      testAlexLogin,
		email:      "alex@example.com",
	}
}

func TestIdentityService_CreateWithID_Errors(t *testing.T) {
	storeErr := errors.New("store failed")
	validID := uuid.New()

	tests := []struct {
		name        string
		userID      uuid.UUID
		modifyInput func(*createInput)
		storeErr    error
		wantErr     error
		wantStore   int
	}{
		{
			name:    "invalid user ID",
			userID:  uuid.Nil(),
			wantErr: ErrInvalidUserID,
		},
		{
			name:   "empty login",
			userID: validID,
			modifyInput: func(i *createInput) {
				i.login = ""
			},
			wantErr: ErrInvalidLogin,
		},
		{
			name:   "whitespace login",
			userID: validID,
			modifyInput: func(i *createInput) {
				i.login = testWhitespace
			},
			wantErr: ErrInvalidLogin,
		},
		{
			name:   "invalid email",
			userID: validID,
			modifyInput: func(i *createInput) {
				i.email = "invalid-email"
			},
			wantErr: ErrInvalidEmail,
		},
		{
			name:   "empty first name",
			userID: validID,
			modifyInput: func(i *createInput) {
				i.firstName = ""
			},
			wantErr: ErrInvalidFirstName,
		},
		{
			name:   "empty last name",
			userID: validID,
			modifyInput: func(i *createInput) {
				i.lastName = testWhitespace
			},
			wantErr: ErrInvalidLastName,
		},
		{
			name:      "store error",
			userID:    validID,
			storeErr:  storeErr,
			wantErr:   storeErr,
			wantStore: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := validCreateInput()
			repo := &fakeIdentityRepository{storeErr: tt.storeErr}

			if tt.modifyInput != nil {
				tt.modifyInput(&input)
			}

			err := NewIdentityService(repo).CreateWithID(
				tt.userID,
				input.firstName,
				input.lastName,
				input.middleName,
				input.login,
				input.email,
			)

			require.ErrorIs(t, err, tt.wantErr)
			assert.Equal(t, tt.wantStore, repo.storeCalls)
			assert.Zero(t, repo.findCalls)
		})
	}
}

func TestIdentityService_FindByLogin(t *testing.T) {
	existingUser := &User{loginSnapshot: testAlexLogin}
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

			user, err := service.FindByLoginSnapshot(tt.login)

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
	current, err := NewUser(userID, "Alex", "Smith", nil, testAlexLogin, Email("alex@example.com"))
	require.NoError(t, err)

	tests := []struct {
		name    string
		first   string
		last    string
		middle  *string
		email   string
		found   *User
		wantErr error
	}{
		{name: "updates profile", first: updatedFirstName, last: updatedLastName, middle: stringPointer("Lee"), email: updatedEmail, found: current},
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
				require.Equal(t, testAlexLogin, repo.updatedUser.loginSnapshot)
				if tt.middle == nil {
					require.Nil(t, repo.updatedUser.middleName)
				} else {
					require.NotNil(t, repo.updatedUser.middleName)
					require.Equal(t, *tt.middle, *repo.updatedUser.middleName)
				}
			}
		})
	}
}

func stringPointer(value string) *string {
	return &value
}
