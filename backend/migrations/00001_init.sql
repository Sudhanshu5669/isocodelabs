-- +goose Up
-- +goose StatementBegin

-- btree_gist lets an exclusion constraint compare plain columns (like an
-- executive id) together with time ranges. Used below to stop one executive
-- from owning two overlapping visit slots.
CREATE EXTENSION IF NOT EXISTS btree_gist;

-- ---------------------------------------------------------------------------
-- Staff accounts and sessions
-- ---------------------------------------------------------------------------
CREATE TABLE users (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email         text NOT NULL,
    name          text NOT NULL,
    role          text NOT NULL CHECK (role IN ('executive', 'head')),
    password_hash text NOT NULL,
    active        boolean NOT NULL DEFAULT true,
    created_at    timestamptz NOT NULL DEFAULT now()
);
-- Emails are unique ignoring case.
CREATE UNIQUE INDEX users_email_key ON users (lower(email));

-- We store only a hash of the session token, so a leaked database does not
-- leak usable logins.
CREATE TABLE sessions (
    token_hash bytea PRIMARY KEY,
    user_id    uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX sessions_user_idx ON sessions (user_id);
CREATE INDEX sessions_expires_idx ON sessions (expires_at);

-- ---------------------------------------------------------------------------
-- Projects and the fact sheet the assistant is allowed to answer from
-- ---------------------------------------------------------------------------
CREATE TABLE projects (
    id          text PRIMARY KEY,           -- slug such as 'project-1'
    name        text NOT NULL,
    rera_number text NOT NULL,
    locality    text NOT NULL,
    address     text NOT NULL,
    latitude    double precision NOT NULL,  -- location pin for site visits
    longitude   double precision NOT NULL
);

CREATE TABLE fact_sheet_entries (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id text NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    topic      text NOT NULL,               -- e.g. 'possession', 'amenities'
    label      text NOT NULL,               -- human title shown in the editor
    value      text NOT NULL,
    updated_by uuid REFERENCES users (id) ON DELETE SET NULL,
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (project_id, topic)
);

-- ---------------------------------------------------------------------------
-- Leads
-- ---------------------------------------------------------------------------
CREATE SEQUENCE lead_code_seq START 60000;

CREATE TABLE leads (
    id                    uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    code                  text NOT NULL UNIQUE DEFAULT ('LD-' || nextval('lead_code_seq')),
    name                  text NOT NULL,
    -- Normalised phone number (E.164). UNIQUE is the hard guarantee behind the
    -- exact-duplicate check: the same number can never become two leads.
    phone                 text NOT NULL UNIQUE,
    source                text NOT NULL CHECK (source IN
                              ('property_portal', 'social_ad', 'website', 'hoarding_call', 'manual')),
    project_id            text REFERENCES projects (id),
    wants                 text CHECK (wants IN ('2 BHK', '3 BHK')),
    budget_min_lakh       integer,
    budget_max_lakh       integer,
    language              text NOT NULL DEFAULT 'en' CHECK (language IN ('en', 'hi', 'gu')),
    status                text NOT NULL DEFAULT 'new' CHECK (status IN
                              ('new', 'engaged', 'visit_booked', 'visited', 'lost')),
    -- DPDP Act: we only contact people who consented, and honour opt-outs.
    consent               boolean NOT NULL DEFAULT false,
    consent_at            timestamptz,
    opted_out             boolean NOT NULL DEFAULT false,
    opted_out_at          timestamptz,
    assigned_to           uuid REFERENCES users (id) ON DELETE SET NULL,
    first_response_at     timestamptz,
    -- Fuzzy duplicates are flagged for a human to confirm, never merged silently.
    possible_duplicate_of uuid REFERENCES leads (id) ON DELETE SET NULL,
    created_at            timestamptz NOT NULL DEFAULT now(),
    updated_at            timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX leads_source_idx ON leads (source);
CREATE INDEX leads_assigned_idx ON leads (assigned_to);
CREATE INDEX leads_created_idx ON leads (created_at DESC);

-- Every enquiry from the same person, with the source it came from. The lead
-- keeps its first source; later touches are recorded here.
CREATE TABLE lead_touches (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    lead_id    uuid NOT NULL REFERENCES leads (id) ON DELETE CASCADE,
    source     text NOT NULL,
    project_id text REFERENCES projects (id),
    note       text,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX lead_touches_lead_idx ON lead_touches (lead_id);

-- ---------------------------------------------------------------------------
-- Conversations (the AI chat) and handoff
-- ---------------------------------------------------------------------------
CREATE TABLE conversations (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    lead_id           uuid NOT NULL REFERENCES leads (id) ON DELETE CASCADE,
    language          text NOT NULL DEFAULT 'en' CHECK (language IN ('en', 'hi', 'gu')),
    summary           text,                 -- AI handoff summary for the executive
    handed_off_at     timestamptz,
    flagged           boolean NOT NULL DEFAULT false,
    flag_reason       text,
    flag_reviewed_at  timestamptz,
    flag_reviewed_by  uuid REFERENCES users (id) ON DELETE SET NULL,
    created_at        timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX conversations_lead_idx ON conversations (lead_id);
CREATE INDEX conversations_flagged_idx ON conversations (flagged) WHERE flagged;

CREATE TABLE messages (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id uuid NOT NULL REFERENCES conversations (id) ON DELETE CASCADE,
    role            text NOT NULL CHECK (role IN ('user', 'assistant', 'system')),
    content         text NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX messages_conversation_idx ON messages (conversation_id, created_at);

-- ---------------------------------------------------------------------------
-- Site-visit slots and visits (the "no double booking" core)
-- ---------------------------------------------------------------------------
CREATE TABLE visit_slots (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id   text NOT NULL REFERENCES projects (id),
    executive_id uuid NOT NULL REFERENCES users (id),
    starts_at    timestamptz NOT NULL,
    ends_at      timestamptz NOT NULL,
    status       text NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'booked', 'cancelled')),
    CHECK (ends_at > starts_at),
    -- One executive cannot own two overlapping slots, even across projects.
    EXCLUDE USING gist (
        executive_id WITH =,
        tstzrange(starts_at, ends_at) WITH &&
    ) WHERE (status <> 'cancelled')
);
CREATE INDEX visit_slots_open_idx ON visit_slots (project_id, starts_at) WHERE status = 'open';

CREATE TABLE visits (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    code       text NOT NULL UNIQUE,        -- unguessable token for the buyer's /visit/:code link
    lead_id    uuid NOT NULL REFERENCES leads (id) ON DELETE CASCADE,
    slot_id    uuid NOT NULL REFERENCES visit_slots (id),
    status     text NOT NULL DEFAULT 'booked' CHECK (status IN ('booked', 'visited', 'no_show', 'cancelled')),
    notes      text,
    outcome_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
-- Safety net #1: a slot can hold at most one live visit, enforced by the database
-- itself, so even a bug in application code cannot double book.
CREATE UNIQUE INDEX visits_one_live_per_slot ON visits (slot_id) WHERE status <> 'cancelled';
-- Safety net #2: a lead can have only one upcoming booked visit at a time.
CREATE UNIQUE INDEX visits_one_booked_per_lead ON visits (lead_id) WHERE status = 'booked';
CREATE INDEX visits_lead_idx ON visits (lead_id);

CREATE TABLE reminders (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    visit_id     uuid NOT NULL REFERENCES visits (id) ON DELETE CASCADE,
    kind         text NOT NULL CHECK (kind IN ('day_before', 'two_hours_before')),
    due_at       timestamptz NOT NULL,
    status       text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'sent', 'skipped', 'failed')),
    skip_reason  text,                      -- e.g. 'opted_out'
    body         text,                      -- the rendered message, includes the RERA number
    sent_at      timestamptz,
    UNIQUE (visit_id, kind)
);
CREATE INDEX reminders_due_idx ON reminders (due_at) WHERE status = 'queued';

-- ---------------------------------------------------------------------------
-- Marketing spend, used for cost per visit on the dashboard
-- ---------------------------------------------------------------------------
CREATE TABLE source_spend (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    source     text NOT NULL,
    month      date NOT NULL,               -- first day of the month
    amount_inr bigint NOT NULL CHECK (amount_inr >= 0),
    UNIQUE (source, month)
);

-- +goose StatementEnd

-- +goose Down
DROP TABLE IF EXISTS source_spend;
DROP TABLE IF EXISTS reminders;
DROP TABLE IF EXISTS visits;
DROP TABLE IF EXISTS visit_slots;
DROP TABLE IF EXISTS messages;
DROP TABLE IF EXISTS conversations;
DROP TABLE IF EXISTS lead_touches;
DROP TABLE IF EXISTS leads;
DROP SEQUENCE IF EXISTS lead_code_seq;
DROP TABLE IF EXISTS fact_sheet_entries;
DROP TABLE IF EXISTS projects;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS users;
