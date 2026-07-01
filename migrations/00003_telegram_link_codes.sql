-- +goose Up
-- +goose StatementBegin
-- One-time codes that bind a Telegram account to an existing web account. The
-- web issues a code for the logged-in user; the bot redeems it with the chat's
-- telegram_id and stamps used_at. Short-lived (see linkCodeTTL). users.telegram_id
-- is already UNIQUE (00001), so one Telegram can't link to two accounts.
CREATE TABLE telegram_link_codes (
    code       text PRIMARY KEY,
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at timestamptz NOT NULL,
    used_at    timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_tg_link_codes_user ON telegram_link_codes (user_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS telegram_link_codes;
-- +goose StatementEnd
