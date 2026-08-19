// Package auth implements registration and login. It depends only on the
// domain ports (UserRepository, PasswordHasher, TokenIssuer) -- never on
// bcrypt or JWT directly (DIP).
package auth

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/geferson/bidcraft/backend/internal/domain"
)

type Service struct {
	users  domain.UserRepository
	hasher domain.PasswordHasher
	tokens domain.TokenIssuer
}

func NewService(users domain.UserRepository, hasher domain.PasswordHasher, tokens domain.TokenIssuer) *Service {
	return &Service{users: users, hasher: hasher, tokens: tokens}
}

type AuthResult struct {
	Token     string
	ExpiresAt time.Time
	User      domain.User
}

func (s *Service) Register(ctx context.Context, email, username, password string) (AuthResult, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	username = strings.TrimSpace(username)

	if email == "" || len(username) < 3 || len(password) < 8 {
		return AuthResult{}, domain.ErrInvalidInput
	}

	hash, err := s.hasher.Hash(password)
	if err != nil {
		return AuthResult{}, err
	}

	u := domain.User{Email: email, Username: username, PasswordHash: hash}
	if err := s.users.Create(ctx, &u); err != nil {
		return AuthResult{}, err
	}

	return s.issue(u)
}

func (s *Service) Login(ctx context.Context, email, password string) (AuthResult, error) {
	email = strings.ToLower(strings.TrimSpace(email))

	u, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		return AuthResult{}, domain.ErrInvalidCredentials
	}
	if err := s.hasher.Compare(u.PasswordHash, password); err != nil {
		return AuthResult{}, domain.ErrInvalidCredentials
	}
	return s.issue(*u)
}

// Me is a thin pass-through so handlers never touch UserRepository
// directly; a JWT-verified id is looked up here for the fresh user record.
func (s *Service) Me(ctx context.Context, userID uuid.UUID) (domain.User, error) {
	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return domain.User{}, err
	}
	return *u, nil
}

func (s *Service) issue(u domain.User) (AuthResult, error) {
	token, expiresAt, err := s.tokens.Issue(domain.AuthUser{ID: u.ID, Username: u.Username})
	if err != nil {
		return AuthResult{}, err
	}
	return AuthResult{Token: token, ExpiresAt: expiresAt, User: u}, nil
}
