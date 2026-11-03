-- Security review fixes for the buyer chat.
--
-- A chat proves only that someone typed a phone number, not that they own it.
-- So a chat may see and change only what it created itself:
--   buyer_name  the name typed in THIS chat (never the stored lead's name)
--   new_lead    true when this chat created the lead; false when the phone
--               already belonged to a lead, in which case the chat may not
--               book or change that lead's details (a person confirms by phone)
--   visit_id    the visit booked in THIS chat, the only visit it may show
-- Chat tokens also expire on the server, not just in the browser.

-- +goose Up
-- +goose StatementBegin
ALTER TABLE conversations
    ADD COLUMN buyer_name text,
    ADD COLUMN new_lead boolean NOT NULL DEFAULT false,
    ADD COLUMN visit_id uuid REFERENCES visits (id) ON DELETE SET NULL,
    ADD COLUMN token_expires_at timestamptz;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE conversations
    DROP COLUMN IF EXISTS token_expires_at,
    DROP COLUMN IF EXISTS visit_id,
    DROP COLUMN IF EXISTS new_lead,
    DROP COLUMN IF EXISTS buyer_name;
-- +goose StatementEnd
