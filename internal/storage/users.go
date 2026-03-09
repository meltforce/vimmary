package storage

import (
	"context"

	"github.com/jackc/pgx/v5"
)

func (db *DB) GetOrCreateUser(ctx context.Context, login, displayName string) (int, error) {
	var id int
	err := db.Pool.QueryRow(ctx, `
		INSERT INTO users (login, display_name)
		VALUES ($1, $2)
		ON CONFLICT (login) DO UPDATE
			SET last_seen = NOW(), display_name = COALESCE(NULLIF($2, ''), users.display_name)
		RETURNING id
	`, login, displayName).Scan(&id)
	return id, err
}

func (db *DB) GetPrimaryUser(ctx context.Context) (id int, login string, err error) {
	err = db.Pool.QueryRow(ctx, `
		SELECT id, login FROM users
		WHERE login LIKE '%@%'
		ORDER BY created_at ASC
		LIMIT 1
	`).Scan(&id, &login)
	if err == pgx.ErrNoRows {
		return 0, "", pgx.ErrNoRows
	}
	return
}
