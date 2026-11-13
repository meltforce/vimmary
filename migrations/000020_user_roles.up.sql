-- Admin and disabled state per user, for application-level login (meltkit
-- pkg/auth). The admin is the row GetPrimaryUser returned until now: the
-- earliest login containing '@'.
ALTER TABLE users ADD COLUMN is_admin BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE users ADD COLUMN disabled_at TIMESTAMPTZ;

UPDATE users SET is_admin = true
WHERE id = (
    SELECT id FROM users
    WHERE login LIKE '%@%'
    ORDER BY created_at ASC
    LIMIT 1
);
