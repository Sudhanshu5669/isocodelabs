-- DPDP: encrypt personal data at rest (step 1 of 3).
--
-- Step 1 (this file) adds an encrypted twin for every personal-data column,
-- plus the blind indexes used to look values up.
-- Step 2 (00006_pii_encrypt.go) encrypts the existing rows into the twins.
-- Step 3 (00007_pii_finish.sql) drops the plaintext columns and gives the
-- twins their final names.
--
-- Every step has a Down, so `migrate down` three times decrypts back to the
-- old plaintext schema (it needs the same keys).

-- +goose Up
-- +goose StatementBegin
ALTER TABLE users
    ADD COLUMN email_enc  bytea,
    ADD COLUMN email_bidx bytea,   -- HMAC of the lower-cased email: sign-in lookup and uniqueness
    ADD COLUMN name_enc   bytea;

ALTER TABLE leads
    ADD COLUMN name_enc   bytea,
    ADD COLUMN phone_enc  bytea,
    ADD COLUMN phone_bidx bytea,   -- HMAC of the normalised phone: dedupe, STOP opt-out, search
    ADD COLUMN name_words bytea[]; -- one HMAC per word of the name: staff search

ALTER TABLE lead_touches  ADD COLUMN note_enc bytea;
ALTER TABLE conversations ADD COLUMN buyer_name_enc bytea, ADD COLUMN summary_enc bytea;
ALTER TABLE messages      ADD COLUMN content_enc bytea;
ALTER TABLE visits        ADD COLUMN notes_enc bytea;
ALTER TABLE reminders     ADD COLUMN body_enc bytea;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE reminders     DROP COLUMN IF EXISTS body_enc;
ALTER TABLE visits        DROP COLUMN IF EXISTS notes_enc;
ALTER TABLE messages      DROP COLUMN IF EXISTS content_enc;
ALTER TABLE conversations DROP COLUMN IF EXISTS summary_enc, DROP COLUMN IF EXISTS buyer_name_enc;
ALTER TABLE lead_touches  DROP COLUMN IF EXISTS note_enc;
ALTER TABLE leads
    DROP COLUMN IF EXISTS name_words,
    DROP COLUMN IF EXISTS phone_bidx,
    DROP COLUMN IF EXISTS phone_enc,
    DROP COLUMN IF EXISTS name_enc;
ALTER TABLE users
    DROP COLUMN IF EXISTS name_enc,
    DROP COLUMN IF EXISTS email_bidx,
    DROP COLUMN IF EXISTS email_enc;
-- +goose StatementEnd
