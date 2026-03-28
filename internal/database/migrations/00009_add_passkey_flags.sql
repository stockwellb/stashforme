-- +goose Up
ALTER TABLE passkeys ADD COLUMN backup_eligible BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE passkeys ADD COLUMN backup_state BOOLEAN NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE passkeys DROP COLUMN backup_state;
ALTER TABLE passkeys DROP COLUMN backup_eligible;
