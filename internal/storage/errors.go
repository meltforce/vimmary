package storage

import "errors"

// ErrNotFound is what a lookup in this package returns when the row does not
// exist, and what an update or delete returns when it matched no row.
//
// It exists so no caller outside this package needs the driver's sentinel.
// CLAUDE.md's layer table places Postgres in internal/storage; before this,
// pgx.ErrNoRows was compared in internal/server, internal/feed, internal/mcp
// and internal/service, which meant three transports and the service layer all
// had to know which driver storage uses.
//
// Callers compare with errors.Is, never with ==. The seven == comparisons this
// replaced were already one wrapper away from being wrong, and one of them was:
// ResummarizeAsync wraps the lookup error with fmt.Errorf("get video: %w"), so
// POST /videos/{id}/resummarize on a missing row answered 500 where the handler
// intended 404.
var ErrNotFound = errors.New("not found")
