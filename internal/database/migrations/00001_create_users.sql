-- +goose Up
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    phone_number VARCHAR(20) NOT NULL UNIQUE,  -- E.164 format (+14155551234)
    display_name VARCHAR(100),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX idx_users_phone_number ON users(phone_number) WHERE deleted_at IS NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_users_phone_number;
DROP TABLE IF EXISTS users;
