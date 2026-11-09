// Package migrations holds the engine's versioned SQLite schema, embedded in every binary.
// SQL files are the only schema source; they apply once each, in name order.
package migrations

import "embed"

// FS holds every *.sql file in this folder.
//
//go:embed *.sql
var FS embed.FS
