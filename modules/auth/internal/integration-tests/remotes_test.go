package integration_tests

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"

	"uuid"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	identityapi "identity/api/server/internalapi"
	sessionapi "session/api/server/internalapi"
)

type remoteServices struct {
	identityAddress  string
	sessionAddress   string
	identityPort     int
	sessionPort      int
	identity         *identityMock
	session          *sessionMock
	identityServer   *grpc.Server
	sessionServer    *grpc.Server
	identityListener net.Listener
	sessionListener  net.Listener
}

func startRemoteServices() (*remoteServices, error) {
	identityListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	sessionListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		_ = identityListener.Close()
		return nil, err
	}

	identityPort, err := listenerPort(identityListener)
	if err != nil {
		_ = identityListener.Close()
		_ = sessionListener.Close()
		return nil, err
	}
	sessionPort, err := listenerPort(sessionListener)
	if err != nil {
		_ = identityListener.Close()
		_ = sessionListener.Close()
		return nil, err
	}

	remotes := &remoteServices{
		identityAddress:  fmt.Sprintf("host.testcontainers.internal:%d", identityPort),
		sessionAddress:   fmt.Sprintf("host.testcontainers.internal:%d", sessionPort),
		identityPort:     identityPort,
		sessionPort:      sessionPort,
		identity:         newIdentityMock(),
		session:          newSessionMock(),
		identityListener: identityListener,
		sessionListener:  sessionListener,
	}
	remotes.identityServer = grpc.NewServer()
	identityapi.RegisterIdentityServiceServer(remotes.identityServer, remotes.identity)
	remotes.sessionServer = grpc.NewServer()
	sessionapi.RegisterSessionServiceServer(remotes.sessionServer, remotes.session)
	go func() { _ = remotes.identityServer.Serve(identityListener) }()
	go func() { _ = remotes.sessionServer.Serve(sessionListener) }()
	return remotes, nil
}

func listenerPort(listener net.Listener) (int, error) {
	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		return 0, err
	}
	var result int
	_, err = fmt.Sscanf(port, "%d", &result)
	return result, err
}

func (r *remoteServices) Close() error {
	r.identityServer.Stop()
	r.sessionServer.Stop()
	return nil
}

type identityMock struct {
	identityapi.UnimplementedIdentityServiceServer
	mu    sync.Mutex
	users map[string]string
}

func newIdentityMock() *identityMock {
	return &identityMock{users: make(map[string]string)}
}

func (s *identityMock) CreateUser(
	_ context.Context,
	request *identityapi.CreateUserRequest,
) (*identityapi.CreateUserResponse, error) {
	if strings.TrimSpace(request.GetFirstName()) == "" ||
		strings.TrimSpace(request.GetLastName()) == "" ||
		strings.TrimSpace(request.GetLogin()) == "" ||
		!strings.Contains(request.GetEmail(), "@") {
		return nil, status.Error(codes.InvalidArgument, "invalid user data")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.users[request.GetLogin()]; exists {
		return nil, status.Error(codes.AlreadyExists, "login already exists")
	}
	userID := uuid.New().String()
	s.users[request.GetLogin()] = userID
	return &identityapi.CreateUserResponse{UserId: userID}, nil
}

func (s *identityMock) FindUserByLogin(
	_ context.Context,
	request *identityapi.FindUserByLoginRequest,
) (*identityapi.FindUserByLoginResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	userID, exists := s.users[request.GetLogin()]
	if !exists {
		return nil, status.Error(codes.NotFound, "user not found")
	}
	return &identityapi.FindUserByLoginResponse{UserId: userID}, nil
}

type sessionMock struct {
	sessionapi.UnimplementedSessionServiceServer
	mu        sync.Mutex
	nextToken int
	sessions  map[string]string
}

func newSessionMock() *sessionMock {
	return &sessionMock{sessions: make(map[string]string)}
}

func (s *sessionMock) CreateSession(
	_ context.Context,
	request *sessionapi.CreateSessionRequest,
) (*sessionapi.CreateSessionResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextToken++
	token := fmt.Sprintf("session-%d", s.nextToken)
	s.sessions[token] = request.GetUserId()
	return &sessionapi.CreateSessionResponse{Token: token}, nil
}

func (s *sessionMock) ResolveSession(
	_ context.Context,
	request *sessionapi.ResolveSessionRequest,
) (*sessionapi.ResolveSessionResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	userID, exists := s.sessions[request.GetToken()]
	if !exists {
		return nil, status.Error(codes.Unauthenticated, "invalid session")
	}
	return &sessionapi.ResolveSessionResponse{UserId: userID}, nil
}

func (s *sessionMock) RevokeSession(
	_ context.Context,
	request *sessionapi.RevokeSessionRequest,
) (*sessionapi.RevokeSessionResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, request.GetToken())
	return &sessionapi.RevokeSessionResponse{}, nil
}

func (s *sessionMock) RevokeOthersByUserID(
	_ context.Context,
	request *sessionapi.RevokeOthersByUserIDRequest,
) (*sessionapi.RevokeOthersByUserIDResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sessions[request.GetToken()] != request.GetUserId() {
		return nil, status.Error(codes.Unauthenticated, "invalid session")
	}
	for token, userID := range s.sessions {
		if userID == request.GetUserId() && token != request.GetToken() {
			delete(s.sessions, token)
		}
	}
	return &sessionapi.RevokeOthersByUserIDResponse{}, nil
}

func (s *sessionMock) sessionCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.sessions)
}

func (s *sessionMock) active(token string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, exists := s.sessions[token]
	return exists
}
