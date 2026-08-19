package http

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
)

type errorEnvelope struct {
	Error     errorBody `json:"error"`
	RequestID string    `json:"request_id,omitempty"`
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeErrorDetails is the ONE place in the codebase that turns a Go error
// into an HTTP response (DRY): it consults errmap.go's single status table,
// falling back to 500 (and logging) for anything not explicitly mapped.
func writeErrorDetails(w http.ResponseWriter, r *http.Request, err error, details any) {
	requestID, _ := r.Context().Value(ctxKeyRequestID).(string)

	if info, ok := lookupError(err); ok {
		writeJSON(w, info.Status, errorEnvelope{
			Error:     errorBody{Code: info.Code, Message: err.Error(), Details: details},
			RequestID: requestID,
		})
		return
	}

	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		writeJSON(w, http.StatusRequestTimeout, errorEnvelope{
			Error:     errorBody{Code: "REQUEST_CANCELLED", Message: "request was cancelled or timed out"},
			RequestID: requestID,
		})
		return
	}

	slog.Default().Error("unhandled error", "err", err, "request_id", requestID, "path", r.URL.Path)
	writeJSON(w, http.StatusInternalServerError, errorEnvelope{
		Error:     errorBody{Code: "INTERNAL_ERROR", Message: "an unexpected error occurred"},
		RequestID: requestID,
	})
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	writeErrorDetails(w, r, err, nil)
}

// bidTooLowDetails lets a 409 BID_TOO_LOW response carry min_next_bid_cents
// so the frontend can auto-correct the input field without a second request.
func bidTooLowDetails(minNextBidCents, currentPriceCents int64) map[string]int64 {
	return map[string]int64{
		"min_next_bid_cents":  minNextBidCents,
		"current_price_cents": currentPriceCents,
	}
}
