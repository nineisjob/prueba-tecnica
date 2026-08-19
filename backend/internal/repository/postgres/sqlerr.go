package postgres

import (
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/geferson/bidcraft/backend/internal/domain"
)

const pgUniqueViolation = "23505"

// classifyWriteErr maps a raw pgx unique-violation to a domain sentinel by
// inspecting which constraint fired, leaving everything else to bubble up
// as-is for the caller to log as unexpected.
func classifyWriteErr(err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
		switch pgErr.ConstraintName {
		case "bids_amount_unique":
			return domain.ErrDuplicateAmount
		case "users_email_key":
			return domain.ErrEmailTaken
		case "users_username_key":
			return domain.ErrUsernameTaken
		}
	}
	return err
}

func isNoRows(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}
