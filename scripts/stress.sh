#!/usr/bin/env bash
# Thin wrapper around cmd/stress so the empirical race-condition proof has a
# one-line entry point. Requires Go locally, OR run the same binary inside
# the backend-test container. See README.md's "Race Conditions" section.
set -euo pipefail
cd "$(dirname "$0")/../backend"
go run ./cmd/stress "$@"
