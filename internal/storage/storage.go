package storage

import (
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
