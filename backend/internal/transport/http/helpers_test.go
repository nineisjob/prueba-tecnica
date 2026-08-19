package http

import (
	"encoding/json"
	"testing"
)

func decodeJSON(t *testing.T, body []byte, v any) {
	t.Helper()
	if err := json.Unmarshal(body, v); err != nil {
		t.Fatalf("failed to decode JSON response: %v\nbody: %s", err, body)
	}
}
