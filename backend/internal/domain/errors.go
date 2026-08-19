package domain

import "errors"

// Sentinel errors. transport/http/errmap.go is the single place that maps
// these to HTTP status codes and machine-readable error codes.
var (
	ErrAuctionNotFound    = errors.New("auction not found")
	ErrUserNotFound       = errors.New("user not found")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUnauthenticated    = errors.New("unauthenticated")
	ErrEmailTaken         = errors.New("email already registered")
	ErrUsernameTaken      = errors.New("username already taken")

	ErrBidTooLow         = errors.New("bid is below the minimum next bid")
	ErrAuctionNotStarted = errors.New("auction has not started yet")
	ErrAuctionEnded      = errors.New("auction has already ended")
	ErrAlreadyHighest    = errors.New("bidder is already the highest bidder")
	ErrEngineBusy        = errors.New("bidding engine is busy, try again")
	ErrDuplicateAmount   = errors.New("an identical bid amount was already accepted")

	ErrInvalidInput = errors.New("invalid input")
)
