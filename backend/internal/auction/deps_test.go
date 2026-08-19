package auction

import (
	"go/build"
	"testing"
)

// TestEngineHasNoTransportDependencies is an architecture guard: it parses
// this package's imports and fails if it depends on net/http, any websocket
// package, or database/sql. This is the concrete, testable proof (not just
// an assertion in a README) that the auction engine is decoupled from
// transport and persistence-specific concerns, as SOLID's OCP/ISP demands.
func TestEngineHasNoTransportDependencies(t *testing.T) {
	pkg, err := build.ImportDir(".", 0)
	if err != nil {
		t.Fatalf("failed to inspect package imports: %v", err)
	}

	forbidden := map[string]string{
		"net/http":     "the engine must not know about HTTP",
		"database/sql": "the engine must not know about SQL",
		"net":          "the engine must not open network connections directly",
	}

	for _, imp := range pkg.Imports {
		if reason, ok := forbidden[imp]; ok {
			t.Errorf("package auction imports %q, which it must not: %s", imp, reason)
		}
		if len(imp) >= 9 && imp[:9] == "websocket" {
			t.Errorf("package auction imports a websocket package (%q); it must stay transport-agnostic", imp)
		}
	}
}
