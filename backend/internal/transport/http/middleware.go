package http

import (
	"bufio"
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/geferson/bidcraft/backend/internal/domain"
)

type ctxKey int

const (
	ctxKeyRequestID ctxKey = iota
	ctxKeyAuthUser
)

type Middleware func(http.Handler) http.Handler

// chain composes middleware so the router stays a plain net/http.ServeMux
// (no routing framework -- the strongest possible YAGNI signal) while
// keeping cross-cutting concerns out of every handler (SRP).
func chain(h http.Handler, mws ...Middleware) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}

func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := uuid.NewString()
		w.Header().Set("X-Request-ID", id)
		ctx := context.WithValue(r.Context(), ctxKeyRequestID, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func RecoverMiddleware(log *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					requestID, _ := r.Context().Value(ctxKeyRequestID).(string)
					log.Error("panic recovered", "panic", rec, "request_id", requestID, "path", r.URL.Path)
					writeJSON(w, http.StatusInternalServerError, errorEnvelope{
						Error:     errorBody{Code: "INTERNAL_ERROR", Message: "an unexpected error occurred"},
						RequestID: requestID,
					})
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

func LoggerMiddleware(log *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(sw, r)
			requestID, _ := r.Context().Value(ctxKeyRequestID).(string)
			log.Info("request",
				"method", r.Method, "path", r.URL.Path, "status", sw.status,
				"duration_ms", time.Since(start).Milliseconds(), "request_id", requestID,
			)
		})
	}
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

// Hijack delegates to the wrapped ResponseWriter's http.Hijacker. Without
// this, the WebSocket upgrade route fails with 501 Not Implemented: the ws
// library type-asserts w.(http.Hijacker) directly (not via
// http.ResponseController's Unwrap chain), and statusWriter would
// otherwise hide that capability from the underlying connection.
func (w *statusWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hj, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("underlying ResponseWriter does not support hijacking")
	}
	return hj.Hijack()
}

func CORSMiddleware(allowedOrigins string) Middleware {
	origins := map[string]bool{}
	for _, o := range strings.Split(allowedOrigins, ",") {
		if o = strings.TrimSpace(o); o != "" {
			origins[o] = true
		}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origins[origin] || origins["*"] {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// JWTAuthMiddleware validates the Authorization: Bearer <token> header and
// injects domain.AuthUser into the request context. Handlers that require
// auth call MustAuthUser; it panics (caught by RecoverMiddleware) if this
// middleware wasn't applied to the route, which is a programmer error, not
// a runtime condition to handle gracefully.
func JWTAuthMiddleware(verifier domain.TokenVerifier) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			token, ok := strings.CutPrefix(header, "Bearer ")
			if !ok || token == "" {
				writeError(w, r, domain.ErrUnauthenticated)
				return
			}
			user, err := verifier.Verify(token)
			if err != nil {
				writeError(w, r, domain.ErrUnauthenticated)
				return
			}
			ctx := context.WithValue(r.Context(), ctxKeyAuthUser, user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func AuthUserFromContext(ctx context.Context) (domain.AuthUser, bool) {
	u, ok := ctx.Value(ctxKeyAuthUser).(domain.AuthUser)
	return u, ok
}
