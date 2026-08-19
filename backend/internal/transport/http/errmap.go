package http

import (
	"errors"
	"net/http"

	"github.com/geferson/bidcraft/backend/internal/domain"
)

type errInfo struct {
	Status int
	Code   string
}

// errStatus is the single source of truth for HTTP semantics: every domain
// sentinel error maps to exactly one (status, code) pair here, so no
// handler ever hardcodes a status code for a business-rule outcome.
var errStatus = map[error]errInfo{
	domain.ErrAuctionNotFound:    {http.StatusNotFound, "AUCTION_NOT_FOUND"},
	domain.ErrUserNotFound:       {http.StatusNotFound, "USER_NOT_FOUND"},
	domain.ErrInvalidCredentials: {http.StatusUnauthorized, "INVALID_CREDENTIALS"},
	domain.ErrUnauthenticated:    {http.StatusUnauthorized, "UNAUTHENTICATED"},
	domain.ErrEmailTaken:         {http.StatusConflict, "EMAIL_TAKEN"},
	domain.ErrUsernameTaken:      {http.StatusConflict, "USERNAME_TAKEN"},
	domain.ErrBidTooLow:          {http.StatusConflict, "BID_TOO_LOW"},
	domain.ErrAuctionNotStarted:  {http.StatusConflict, "AUCTION_NOT_STARTED"},
	domain.ErrAuctionEnded:       {http.StatusConflict, "AUCTION_ENDED"},
	domain.ErrAlreadyHighest:     {http.StatusConflict, "ALREADY_HIGHEST_BIDDER"},
	domain.ErrDuplicateAmount:    {http.StatusConflict, "DUPLICATE_AMOUNT"},
	domain.ErrEngineBusy:         {http.StatusServiceUnavailable, "ENGINE_BUSY"},
	domain.ErrInvalidInput:       {http.StatusBadRequest, "INVALID_INPUT"},
}

func lookupError(err error) (errInfo, bool) {
	for sentinel, info := range errStatus {
		if errors.Is(err, sentinel) {
			return info, true
		}
	}
	return errInfo{}, false
}
