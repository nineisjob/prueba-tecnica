// Package migrations embeds the .sql migration files into the binary so
// `docker compose up` needs no separate migration step or CLI tool.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
