-- DPDP: encrypt personal data at rest (step 3 of 3).
--
-- The encrypted twins now hold every value, so the plaintext columns go and the
-- twins take their names. The database's own guarantees move to the blind
-- indexes: one user per email, one lead per phone number.

-- +goose Up
-- +goose StatementBegin
ALTER TABLE users DROP COLUMN email, DROP COLUMN name;   -- also drops users_email_key on lower(email)
ALTER TABLE users RENAME COLUMN email_enc TO email;
ALTER TABLE users RENAME COLUMN name_enc TO name;
ALTER TABLE users
    ALTER COLUMN email SET NOT NULL,
    ALTER COLUMN email_bidx SET NOT NULL,
    ALTER COLUMN name SET NOT NULL;
CREATE UNIQUE INDEX users_email_key ON users (email_bidx);

ALTER TABLE leads DROP COLUMN name, DROP COLUMN phone;   -- also drops the UNIQUE (phone) constraint
ALTER TABLE leads RENAME COLUMN name_enc TO name;
ALTER TABLE leads RENAME COLUMN phone_enc TO phone;
ALTER TABLE leads
    ALTER COLUMN name SET NOT NULL,
    ALTER COLUMN phone SET NOT NULL,
    ALTER COLUMN phone_bidx SET NOT NULL,
    ALTER COLUMN name_words SET NOT NULL,
    ALTER COLUMN name_words SET DEFAULT '{}';
-- The hard guarantee behind "one person is one lead", now on the blind index.
ALTER TABLE leads ADD CONSTRAINT leads_phone_bidx_key UNIQUE (phone_bidx);
CREATE INDEX leads_name_words_idx ON leads USING gin (name_words);

ALTER TABLE lead_touches DROP COLUMN note;
ALTER TABLE lead_touches RENAME COLUMN note_enc TO note;

ALTER TABLE conversations DROP COLUMN buyer_name, DROP COLUMN summary;
ALTER TABLE conversations RENAME COLUMN buyer_name_enc TO buyer_name;
ALTER TABLE conversations RENAME COLUMN summary_enc TO summary;

ALTER TABLE messages DROP COLUMN content;
ALTER TABLE messages RENAME COLUMN content_enc TO content;
ALTER TABLE messages ALTER COLUMN content SET NOT NULL;

ALTER TABLE visits DROP COLUMN notes;
ALTER TABLE visits RENAME COLUMN notes_enc TO notes;

ALTER TABLE reminders DROP COLUMN body;
ALTER TABLE reminders RENAME COLUMN body_enc TO body;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Back to the shape step 2 expects: encrypted twins beside empty plaintext
-- columns. Step 2's Down then decrypts into them.
ALTER TABLE reminders RENAME COLUMN body TO body_enc;
ALTER TABLE reminders ADD COLUMN body text;

ALTER TABLE visits RENAME COLUMN notes TO notes_enc;
ALTER TABLE visits ADD COLUMN notes text;

ALTER TABLE messages ALTER COLUMN content DROP NOT NULL;
ALTER TABLE messages RENAME COLUMN content TO content_enc;
ALTER TABLE messages ADD COLUMN content text;

ALTER TABLE conversations RENAME COLUMN summary TO summary_enc;
ALTER TABLE conversations RENAME COLUMN buyer_name TO buyer_name_enc;
ALTER TABLE conversations ADD COLUMN buyer_name text, ADD COLUMN summary text;

ALTER TABLE lead_touches RENAME COLUMN note TO note_enc;
ALTER TABLE lead_touches ADD COLUMN note text;

DROP INDEX IF EXISTS leads_name_words_idx;
ALTER TABLE leads DROP CONSTRAINT IF EXISTS leads_phone_bidx_key;
ALTER TABLE leads
    ALTER COLUMN name DROP NOT NULL,
    ALTER COLUMN phone DROP NOT NULL,
    ALTER COLUMN phone_bidx DROP NOT NULL,
    ALTER COLUMN name_words DROP NOT NULL,
    ALTER COLUMN name_words DROP DEFAULT;
ALTER TABLE leads RENAME COLUMN name TO name_enc;
ALTER TABLE leads RENAME COLUMN phone TO phone_enc;
ALTER TABLE leads ADD COLUMN name text, ADD COLUMN phone text;

DROP INDEX IF EXISTS users_email_key;
ALTER TABLE users
    ALTER COLUMN email DROP NOT NULL,
    ALTER COLUMN email_bidx DROP NOT NULL,
    ALTER COLUMN name DROP NOT NULL;
ALTER TABLE users RENAME COLUMN email TO email_enc;
ALTER TABLE users RENAME COLUMN name TO name_enc;
ALTER TABLE users ADD COLUMN email text, ADD COLUMN name text;
-- +goose StatementEnd
