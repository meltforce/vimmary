package storage

import (
	"context"

	"github.com/meltforce/meltkit/pkg/db"
)

// DB wraps meltkit's DB to allow defining storage methods in this package.
type DB struct {
	*db.DB
}

// NewDB wraps a meltkit db.DB for use with storage query methods.
func NewDB(d *db.DB) *DB {
	return &DB{DB: d}
}

// Ping reports whether the database answers. Used by the health endpoint, so
// an unreachable database shows up as unhealthy rather than as a service that
// accepts requests and fails every one of them.
func (db *DB) Ping(ctx context.Context) error {
	return db.Pool.Ping(ctx)
}
