-- +goose Up
CREATE TABLE verification_codes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    phone_number VARCHAR(20) NOT NULL,
    code VARCHAR(6) NOT NULL,
    attempts INTEGER NOT NULL DEFAULT 0,
    expires_at TIMESTAMPTZ NOT NULL,
    verified_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_verification_codes_phone_expires
    ON verification_codes(phone_number, expires_at)
    WHERE verified_at IS NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_verification_codes_phone_expires;
DROP TABLE IF EXISTS verification_codes;
