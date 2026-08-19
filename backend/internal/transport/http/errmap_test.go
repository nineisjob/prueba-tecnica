package http

import (
	"net/http/httptest"
	"testing"
)

// TestEveryDomainErrorMapsToDocumentedStatus walks the single errStatus
// table and asserts writeError produces exactly the documented status and
// machine-readable code for every sentinel -- this table IS the contract
// from the API doc (section 5.1), so this test is what keeps code and doc
// from silently drifting apart.
func TestEveryDomainErrorMapsToDocumentedStatus(t *testing.T) {
	for sentinel, info := range errStatus {
		t.Run(info.Code, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/whatever", nil)
			rec := httptest.NewRecorder()
			writeError(rec, req, sentinel)

			if rec.Code != info.Status {
				t.Fatalf("status = %d, want %d", rec.Code, info.Status)
			}

			var body errorEnvelope
			decodeJSON(t, rec.Body.Bytes(), &body)
			if body.Error.Code != info.Code {
				t.Fatalf("error.code = %q, want %q", body.Error.Code, info.Code)
			}
		})
	}
}

func TestUnmappedErrorReturns500(t *testing.T) {
	req := httptest.NewRequest("GET", "/whatever", nil)
	rec := httptest.NewRecorder()
	writeError(rec, req, errUnmapped)

	if rec.Code != 500 {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	var body errorEnvelope
	decodeJSON(t, rec.Body.Bytes(), &body)
	if body.Error.Code != "INTERNAL_ERROR" {
		t.Fatalf("error.code = %q, want INTERNAL_ERROR", body.Error.Code)
	}
}

var errUnmapped = domainErrForTest{"something unexpected"}

type domainErrForTest struct{ msg string }

func (e domainErrForTest) Error() string { return e.msg }
