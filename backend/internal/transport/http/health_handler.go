package http

import (
	"context"
	"net/http"
	"time"
)

type Pinger interface {
	Ping(ctx context.Context) error
}

func HealthzHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func ReadyzHandler(pinger Pinger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := pinger.Ping(ctx); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "db unavailable"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	}
}

func ServerTimeHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, serverTimeResponse{ServerTimeMs: time.Now().UTC().UnixMilli()})
}
