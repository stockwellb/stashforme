-- +goose Up
CREATE TABLE lists (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(100) NOT NULL,
    description TEXT,
    is_default BOOLEAN NOT NULL DEFAULT FALSE,
    position INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX idx_lists_user_id ON lists(user_id) WHERE deleted_at IS NULL;

-- Ensure only one default list per user
CREATE UNIQUE INDEX idx_lists_user_default ON lists(user_id) WHERE is_default = TRUE AND deleted_at IS NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_lists_user_default;
DROP INDEX IF EXISTS idx_lists_user_id;
DROP TABLE IF EXISTS lists;
