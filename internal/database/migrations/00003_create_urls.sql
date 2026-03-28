-- +goose Up
CREATE TABLE urls (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    url_hash VARCHAR(64) NOT NULL UNIQUE,  -- SHA256 of normalized URL
    url TEXT NOT NULL,
    title VARCHAR(500),
    description TEXT,
    favicon_url TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_urls_url_hash ON urls(url_hash);

-- +goose Down
DROP INDEX IF EXISTS idx_urls_url_hash;
DROP TABLE IF EXISTS urls;
