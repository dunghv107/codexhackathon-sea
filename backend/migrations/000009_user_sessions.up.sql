BEGIN;

CREATE TABLE IF NOT EXISTS user_sessions (
    id uuid PRIMARY KEY,
    user_id bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    refresh_token_hash char(64) NOT NULL UNIQUE,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_user_sessions_user_expiry ON user_sessions (user_id, expires_at);

-- Legacy access tokens cannot identify a device; require one fresh login after upgrade.
-- Clearing their hashes also prevents a rollback from reviving these sessions.
UPDATE users SET refresh_token_hash = NULL, refresh_token_expires_at = NULL;

COMMIT;
