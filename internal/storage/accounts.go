package storage

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// UserAccount is a row of `users` as account management sees it.
type UserAccount struct {
	ID          int        `json:"id"`
	Login       string     `json:"login"`
	DisplayName string     `json:"display_name"`
	IsAdmin     bool       `json:"is_admin"`
	DisabledAt  *time.Time `json:"disabled_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	LastSeen    time.Time  `json:"last_seen"`
	// Items counts the user's rows in `videos`, of both sources.
	Items int `json:"items"`
}

// Disabled reports whether the account is disabled.
func (u UserAccount) Disabled() bool { return u.DisabledAt != nil }

const userAccountColumns = `u.id, u.login, COALESCE(u.display_name, ''), u.is_admin, u.disabled_at,
	u.created_at, u.last_seen, (SELECT count(*) FROM videos v WHERE v.user_id = u.id)`

func scanUserAccount(row pgx.Row) (UserAccount, error) {
	var a UserAccount
	err := row.Scan(&a.ID, &a.Login, &a.DisplayName, &a.IsAdmin, &a.DisabledAt,
		&a.CreatedAt, &a.LastSeen, &a.Items)
	if errors.Is(err, pgx.ErrNoRows) {
		return UserAccount{}, ErrNotFound
	}
	return a, err
}

// ListUserAccounts returns every user, oldest first.
func (db *DB) ListUserAccounts(ctx context.Context) ([]UserAccount, error) {
	rows, err := db.Pool.Query(ctx, `SELECT `+userAccountColumns+` FROM users u ORDER BY u.created_at, u.id`)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()
	var out []UserAccount
	for rows.Next() {
		a, err := scanUserAccount(rows)
		if err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// GetUserAccount returns one user or ErrNotFound.
func (db *DB) GetUserAccount(ctx context.Context, id int) (UserAccount, error) {
	a, err := scanUserAccount(db.Pool.QueryRow(ctx,
		`SELECT `+userAccountColumns+` FROM users u WHERE u.id = $1`, id))
	if err != nil && !errors.Is(err, ErrNotFound) {
		return UserAccount{}, fmt.Errorf("get user %d: %w", id, err)
	}
	return a, err
}

// GetUserIDByLogin returns the user whose login equals login, or ErrNotFound.
func (db *DB) GetUserIDByLogin(ctx context.Context, login string) (int, error) {
	var id int
	err := db.Pool.QueryRow(ctx, `SELECT id FROM users WHERE login = $1`, login).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("get user by login: %w", err)
	}
	return id, nil
}

// CreateUser inserts a user. A taken login returns ErrConflict.
func (db *DB) CreateUser(ctx context.Context, login, displayName string, isAdmin bool) (int, error) {
	var id int
	err := db.Pool.QueryRow(ctx, `
		INSERT INTO users (login, display_name, is_admin)
		VALUES ($1, NULLIF($2, ''), $3)
		RETURNING id
	`, login, displayName, isAdmin).Scan(&id)
	if isPgCode(err, "23505") {
		return 0, ErrConflict
	}
	if err != nil {
		return 0, fmt.Errorf("create user: %w", err)
	}
	return id, nil
}

// AdminExists reports whether any user, disabled or not, holds the admin flag.
func (db *DB) AdminExists(ctx context.Context) (bool, error) {
	var ok bool
	err := db.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM users WHERE is_admin)`).Scan(&ok)
	if err != nil {
		return false, fmt.Errorf("check admin: %w", err)
	}
	return ok, nil
}

// PromoteFirstAdmin sets the admin flag on user id if no user holds it. It
// reports whether it did.
func (db *DB) PromoteFirstAdmin(ctx context.Context, id int) (bool, error) {
	tag, err := db.Pool.Exec(ctx, `
		UPDATE users SET is_admin = true
		WHERE id = $1 AND NOT EXISTS (SELECT 1 FROM users WHERE is_admin)
	`, id)
	if err != nil {
		return false, fmt.Errorf("promote admin: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// SetUserAdmin sets or clears the admin flag. ErrNotFound if no such user.
func (db *DB) SetUserAdmin(ctx context.Context, id int, admin bool) error {
	return affectedOne(db.Pool.Exec(ctx, `UPDATE users SET is_admin = $2 WHERE id = $1`, id, admin))
}

// SetUserDisabled disables or re-enables a user. ErrNotFound if no such user.
func (db *DB) SetUserDisabled(ctx context.Context, id int, disabled bool) error {
	return affectedOne(db.Pool.Exec(ctx, `
		UPDATE users SET disabled_at = CASE WHEN $2 THEN COALESCE(disabled_at, NOW()) END
		WHERE id = $1
	`, id, disabled))
}

// GetUserFlags returns the admin flag and the disabled state of a user.
// ErrNotFound if no such user.
func (db *DB) GetUserFlags(ctx context.Context, id int) (isAdmin, disabled bool, err error) {
	err = db.Pool.QueryRow(ctx,
		`SELECT is_admin, disabled_at IS NOT NULL FROM users WHERE id = $1`, id).Scan(&isAdmin, &disabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, false, ErrNotFound
	}
	if err != nil {
		return false, false, fmt.Errorf("get user flags: %w", err)
	}
	return isAdmin, disabled, nil
}

// TouchUser records a login on the user row.
func (db *DB) TouchUser(ctx context.Context, id int, displayName string) error {
	_, err := db.Pool.Exec(ctx, `
		UPDATE users SET last_seen = NOW(),
			display_name = COALESCE(NULLIF($2, ''), display_name)
		WHERE id = $1
	`, id, displayName)
	return err
}

// DeleteUser removes a user that owns no `videos` rows; channel follows,
// inbox items and podcast subscriptions go with it (ON DELETE CASCADE). A user
// with library rows returns ErrConflict, because those rows reference the user
// without a cascade and carry the shared transcripts and summaries.
func (db *DB) DeleteUser(ctx context.Context, id int) error {
	err := affectedOne(db.Pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, id))
	if isPgCode(err, "23503") {
		return ErrConflict
	}
	return err
}

func affectedOne(tag pgconn.CommandTag, err error) error {
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func isPgCode(err error, code string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == code
}
