package vimmary

import "embed"

// MigrationsFS contains the embedded SQL migration files.
//
//go:embed migrations/*.sql
var MigrationsFS embed.FS
