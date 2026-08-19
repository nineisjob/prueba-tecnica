package http

import (
	"log/slog"
	"net/http"

	"github.com/geferson/bidcraft/backend/internal/domain"
)

type RouterDeps struct {
	Auth          *AuthHandler
	Auctions      *AuctionHandler
	TokenVerifier domain.TokenVerifier
	Pinger        Pinger
	Log           *slog.Logger
	CORSOrigins   string
	WS            http.Handler // set by transport/ws; kept as a plain http.Handler here to avoid an import cycle
}

// NewRouter wires every endpoint from the API contract onto the stdlib
// ServeMux (Go 1.22+ method+wildcard patterns: "POST /path/{id}"). No
// routing framework -- the strongest possible YAGNI signal for a project
// this size, and chain() (12 lines, middleware.go) covers everything a
// framework's middleware stack would.
func NewRouter(d RouterDeps) http.Handler {
	mux := http.NewServeMux()

	authRequired := JWTAuthMiddleware(d.TokenVerifier)

	mux.HandleFunc("POST /api/v1/auth/register", d.Auth.Register)
	mux.HandleFunc("POST /api/v1/auth/login", d.Auth.Login)
	mux.Handle("GET /api/v1/auth/me", authRequired(http.HandlerFunc(d.Auth.Me)))

	mux.HandleFunc("GET /api/v1/auctions", d.Auctions.List)
	mux.Handle("POST /api/v1/auctions", authRequired(http.HandlerFunc(d.Auctions.Create)))
	mux.HandleFunc("GET /api/v1/auctions/{id}", d.Auctions.Get)
	mux.HandleFunc("GET /api/v1/auctions/{id}/bids", d.Auctions.ListBids)
	mux.Handle("POST /api/v1/auctions/{id}/bids", authRequired(http.HandlerFunc(d.Auctions.PlaceBid)))

	if d.WS != nil {
		mux.Handle("GET /api/v1/auctions/{id}/ws", d.WS)
	}

	mux.HandleFunc("GET /api/v1/time", ServerTimeHandler)
	mux.HandleFunc("GET /healthz", HealthzHandler)
	mux.HandleFunc("GET /readyz", ReadyzHandler(d.Pinger))

	return chain(mux,
		RequestIDMiddleware,
		RecoverMiddleware(d.Log),
		LoggerMiddleware(d.Log),
		CORSMiddleware(d.CORSOrigins),
	)
}
