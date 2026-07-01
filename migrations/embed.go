// Package migrations embeds the goose SQL migrations so the migrate binary can
// apply them without the goose CLI or the files on disk (identical behaviour
// locally and in the container).
package migrations

import "embed"

// FS holds the *.sql goose migrations.
//
//go:embed *.sql
var FS embed.FS
