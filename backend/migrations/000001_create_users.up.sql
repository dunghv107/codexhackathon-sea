BEGIN;

CREATE TABLE users (
    id bigserial PRIMARY KEY,
    username varchar(50) NOT NULL,
    email varchar(255) NOT NULL,
    password varchar(255) NOT NULL,
    role varchar(20) NOT NULL DEFAULT 'USER',
    status varchar(20) NOT NULL DEFAULT 'ACTIVE',
    refresh_token_hash char(64),
    refresh_token_expires_at timestamptz,
    created_at timestamptz,
    updated_at timestamptz,
    deleted_at timestamptz
);

CREATE UNIQUE INDEX idx_users_username ON users (username);
CREATE UNIQUE INDEX idx_users_email ON users (email);
CREATE UNIQUE INDEX idx_users_refresh_token_hash ON users (refresh_token_hash);
CREATE INDEX idx_users_deleted_at ON users (deleted_at);

COMMIT;
