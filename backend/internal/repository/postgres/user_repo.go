package postgres

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"github.com/geferson/bidcraft/backend/internal/domain"
)

func (r *UserRepo) Create(ctx context.Context, u *domain.User) error {
	row := r.pool.QueryRow(ctx, `
		INSERT INTO users (email, username, password_hash)
		VALUES ($1, $2, $3)
		RETURNING id, email, username, password_hash, created_at`,
		strings.ToLower(u.Email), u.Username, u.PasswordHash,
	)
	var created domain.User
	if err := row.Scan(&created.ID, &created.Email, &created.Username, &created.PasswordHash, &created.CreatedAt); err != nil {
		return classifyWriteErr(err)
	}
	*u = created
	return nil
}

func (r *UserRepo) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, email, username, password_hash, created_at
		FROM users WHERE email = $1`, strings.ToLower(email))
	var u domain.User
	if err := row.Scan(&u.ID, &u.Email, &u.Username, &u.PasswordHash, &u.CreatedAt); err != nil {
		if isNoRows(err) {
			return nil, domain.ErrUserNotFound
		}
		return nil, err
	}
	return &u, nil
}

func (r *UserRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, email, username, password_hash, created_at
		FROM users WHERE id = $1`, id)
	var u domain.User
	if err := row.Scan(&u.ID, &u.Email, &u.Username, &u.PasswordHash, &u.CreatedAt); err != nil {
		if isNoRows(err) {
			return nil, domain.ErrUserNotFound
		}
		return nil, err
	}
	return &u, nil
}
