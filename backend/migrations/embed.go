// Package migrations bundles the SQL migration files into the Go binary.
//
// Go lesson: `//go:embed` copies files into the compiled program, so the
// deployed binary carries its own migrations and needs no extra files.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
