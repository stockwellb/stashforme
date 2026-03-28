-- +goose Up
CREATE TABLE list_urls (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    list_id UUID NOT NULL REFERENCES lists(id) ON DELETE CASCADE,
    url_id UUID NOT NULL REFERENCES urls(id) ON DELETE CASCADE,
    notes TEXT,
    position INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    UNIQUE(list_id, url_id)
);

CREATE INDEX idx_list_urls_list_id ON list_urls(list_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_list_urls_url_id ON list_urls(url_id) WHERE deleted_at IS NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_list_urls_url_id;
DROP INDEX IF EXISTS idx_list_urls_list_id;
DROP TABLE IF EXISTS list_urls;
