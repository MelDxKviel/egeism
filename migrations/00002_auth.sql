-- +goose Up
-- +goose StatementBegin
-- Credential login. telegram_id users authenticate without a password; web
-- users log in with username + password. Both nullable so either path works.
ALTER TABLE users ADD COLUMN username      text UNIQUE;
ALTER TABLE users ADD COLUMN password_hash text;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE users DROP COLUMN IF EXISTS password_hash;
ALTER TABLE users DROP COLUMN IF EXISTS username;
-- +goose StatementEnd
