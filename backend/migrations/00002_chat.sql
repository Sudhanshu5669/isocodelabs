-- Phase 4: the buyer chat.
--
-- Buyers do not log in. Starting a chat gives the browser a random token in a
-- cookie; we keep only its hash here (like staff sessions), and that token is
-- the only thing that lets someone continue the chat or book through it.

-- +goose Up
-- +goose StatementBegin
ALTER TABLE conversations
    ADD COLUMN token_hash bytea UNIQUE,
    ADD COLUMN project_id text REFERENCES projects (id),
    ADD COLUMN updated_at timestamptz NOT NULL DEFAULT now();

-- The third qualifying question (after apartment type and budget): when the
-- buyer wants to move or buy.
ALTER TABLE leads
    ADD COLUMN timeline text CHECK (timeline IN
        ('within_3_months', '3_to_6_months', '6_to_12_months', 'over_12_months', 'exploring'));
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE leads DROP COLUMN IF EXISTS timeline;
ALTER TABLE conversations
    DROP COLUMN IF EXISTS updated_at,
    DROP COLUMN IF EXISTS project_id,
    DROP COLUMN IF EXISTS token_hash;
-- +goose StatementEnd
