package http

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/google/uuid"

	"github.com/geferson/bidcraft/backend/internal/auth"
	"github.com/geferson/bidcraft/backend/internal/domain"
)

// AuthService is the narrow interface this handler needs (ISP): it depends
// on *auth.Service's public surface, not on UserRepository/PasswordHasher/
// TokenIssuer individually, so it can be swapped for a fake in handler tests.
type AuthService interface {
	Register(ctx context.Context, email, username, password string) (auth.AuthResult, error)
	Login(ctx context.Context, email, password string) (auth.AuthResult, error)
	Me(ctx context.Context, userID uuid.UUID) (domain.User, error)
}

type AuthHandler struct {
	svc AuthService
}

func NewAuthHandler(svc AuthService) *AuthHandler {
	return &AuthHandler{svc: svc}
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, r, domain.ErrInvalidInput)
		return
	}
	res, err := h.svc.Register(r.Context(), req.Email, req.Username, req.Password)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, authResponse{Token: res.Token, ExpiresAt: res.ExpiresAt, User: toUserDTO(res.User)})
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, r, domain.ErrInvalidInput)
		return
	}
	res, err := h.svc.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, authResponse{Token: res.Token, ExpiresAt: res.ExpiresAt, User: toUserDTO(res.User)})
}

func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	authUser, ok := AuthUserFromContext(r.Context())
	if !ok {
		writeError(w, r, domain.ErrUnauthenticated)
		return
	}
	u, err := h.svc.Me(r.Context(), authUser.ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]userDTO{"user": toUserDTO(u)})
}
