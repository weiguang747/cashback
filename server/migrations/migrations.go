// Package migrations embeds the goose SQL migrations into the binary.
// Never edit a migration that has shipped; add a new numbered file instead.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
