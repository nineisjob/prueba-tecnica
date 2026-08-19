#!/usr/bin/env pwsh
# Thin wrapper around cmd/stress so the empirical race-condition proof has a
# one-line entry point. Requires Go locally, OR run the same binary inside
# the backend-test container. See README.md's "Race Conditions" section.
$ErrorActionPreference = "Stop"
$backendDir = Join-Path $PSScriptRoot "..\backend"
Push-Location $backendDir
try {
    go run ./cmd/stress @args
} finally {
    Pop-Location
}
