package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/geferson/bidcraft/backend/internal/auth"
	"github.com/geferson/bidcraft/backend/internal/domain"
)

type fakeAuthService struct {
	result auth.AuthResult
	err    error
	user   domain.User
}

func (f *fakeAuthService) Register(ctx context.Context, email, username, password string) (auth.AuthResult, error) {
	return f.result, f.err
}
func (f *fakeAuthService) Login(ctx context.Context, email, password string) (auth.AuthResult, error) {
	return f.result, f.err
}
func (f *fakeAuthService) Me(ctx context.Context, userID uuid.UUID) (domain.User, error) {
	return f.user, f.err
}

func TestAuthHandler_Register_Success(t *testing.T) {
	svc := &fakeAuthService{result: auth.AuthResult{
		Token: "tok", ExpiresAt: time.Now().Add(time.Hour),
		User: domain.User{ID: uuid.New(), Username: "alice", Email: "alice@bidcraft.dev"},
	}}
	h := NewAuthHandler(svc)

	body, _ := json.Marshal(registerRequest{Email: "alice@bidcraft.dev", Username: "alice", Password: "demo1234"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	h.Register(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body=%s", rec.Code, rec.Body.String())
	}
	var resp authResponse
	decodeJSON(t, rec.Body.Bytes(), &resp)
	if resp.Token != "tok" || resp.User.Username != "alice" {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestAuthHandler_Register_EmailTaken(t *testing.T) {
	svc := &fakeAuthService{err: domain.ErrEmailTaken}
	h := NewAuthHandler(svc)

	body, _ := json.Marshal(registerRequest{Email: "alice@bidcraft.dev", Username: "alice", Password: "demo1234"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	h.Register(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rec.Code)
	}
}

func TestAuthHandler_Login_InvalidCredentials(t *testing.T) {
	svc := &fakeAuthService{err: domain.ErrInvalidCredentials}
	h := NewAuthHandler(svc)

	body, _ := json.Marshal(loginRequest{Email: "alice@bidcraft.dev", Password: "wrong"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	h.Login(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestAuthHandler_Me_RequiresAuth(t *testing.T) {
	h := NewAuthHandler(&fakeAuthService{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	rec := httptest.NewRecorder()

	h.Me(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}
