// Package jwt implements domain.TokenIssuer and domain.TokenVerifier with
// HS256 (a single shared secret is enough for this single-service scope;
// asymmetric signing would be YAGNI).
package jwt

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/geferson/bidcraft/backend/internal/domain"
)

type Service struct {
	secret []byte
	ttl    time.Duration
}

func NewService(secret string, ttlHours int) Service {
	return Service{secret: []byte(secret), ttl: time.Duration(ttlHours) * time.Hour}
}

type claims struct {
	Username string `json:"username"`
	jwt.RegisteredClaims
}

func (s Service) Issue(u domain.AuthUser) (string, time.Time, error) {
	expiresAt := time.Now().Add(s.ttl)
	c := claims{
		Username: u.Username,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   u.ID.String(),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, c)
	signed, err := token.SignedString(s.secret)
	if err != nil {
		return "", time.Time{}, err
	}
	return signed, expiresAt, nil
}

func (s Service) Verify(tokenString string) (domain.AuthUser, error) {
	var c claims
	token, err := jwt.ParseWithClaims(tokenString, &c, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return s.secret, nil
	})
	if err != nil || !token.Valid {
		return domain.AuthUser{}, domain.ErrUnauthenticated
	}

	id, err := uuid.Parse(c.Subject)
	if err != nil {
		return domain.AuthUser{}, domain.ErrUnauthenticated
	}
	return domain.AuthUser{ID: id, Username: c.Username}, nil
}
