package migrations

import "embed"

// FS holds the embedded SQL migration files so they can be applied at runtime
// without shipping the migrations directory alongside the binary.
//
//go:embed *.sql
var FS embed.FS
