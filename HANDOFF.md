# Session handoff

Read this first in a new session. It is written so someone with no memory of the earlier conversation can carry on. Last updated 2026-11-03, after Phase 3. A repo map for human readers is in `README.md`.

## 1. The assignment (brief, condensed)

A hiring assessment: pick one problem statement, build it end to end, submit **one ZIP with two files**: a **Technical Design Document (PDF or Word)** and a **deployment.md**. Solo, **7 days** from when the step opened. Scored: quality of work 50%, problem solving 30%, communication 20%.

Must have: main flows work on realistic sample data (no dead buttons); frontend, backend, database, sign-in with roles; the AI part really works **including when it is wrong or unsure**; polished UI (consistent design, works on a phone, loading/empty/error states); tests for the key logic; basic security (no secrets in repo, validated inputs, access control).

Rules: use only the information in the problem statement and **label every assumption in the TDD**; **keep the commit history (they read it)**; no secrets or real personal data in the repo; **keep the repo public until reviewed**. The TDD must start with the **GitHub URL** (plus live URL if deployed) and cover: architecture and why, data model, main flows, how the AI works, UI/design decisions and key screens, what was cut and what is next. deployment.md: local run steps, how it is deployed and env var **names only**, demo login details.

### The chosen problem: lead-to-visit AI assistant for "Company A" (Ahmedabad residential developer)
- 2 live projects, 420 apartments (2 and 3 BHK, ₹65 lakh to ₹1.2 crore). ~3,000 leads/month, ₹1,000 per lead, median first response 14 h, 35% never reached in 48 h, only 6% visit a site.
- Ask: answer every enquiry within 2 minutes, any hour, in English, Hindi or Gujarati; answer basic questions from a **fact sheet**; ask budget and needs; **book a site visit in a real free slot**; hand serious buyers to a **named salesperson with a chat summary**.
- Must-have list: one lead list from all sources with source tags and duplicate checks; AI chat assistant (web now, WhatsApp later); site-visit booking with location pin and reminders **1 day and 2 hours before**; handoff with summary; dashboard of response time, visits, show-up rate and **cost per visit by source**; admin editor for fact sheet and visit slots.
- **Core build for software engineers (about 6 to 8 screens):** lead list with duplicate check, AI chat from the fact sheet, site-visit booking with reminders, handoff, source dashboard. Roles: buyer and sales executive. AI: chat assistant with a booking tool. **Hard part: book only real free slots, no double booking.** Mock payments, messages and outside systems.
- AI rules: only state facts from the sales head's fact sheet; if a fact is missing say a person will confirm and hand off; **never quote a final price or discount**; buyers can ask for a person any time; the sales head reviews flagged chats daily.
- Constraints: RERA (promotional messages show the project registration number, no claims beyond registered details); DPDP Act 2023 (contact only with consent, honour opt-outs); WhatsApp needs opt-in templates.
- Design: client wants good, minimal design: white space, restrained palette, clear type, nothing without purpose; chat and booking should feel calm and premium. Design counts as much as features.
- Success: median first response under 5 minutes; unreached-in-48h from 35% to under 5%; lead-to-visit 6% to 8% in 3 months. Budget ₹6 lakh, 6 weeks.
- Sample leads: LD-58210 Hardik Patel, Property portal, Project 1, 3 BHK, Booked 2026-11-01 11:00 · LD-58211 Nidhi Shah, Social media ad, Project 2, 2 BHK, Not booked · LD-58224 Kavita Joshi, Hoarding call, Project 2, 2 BHK, Visited.
- Out of scope: full CRM replacement, booking payments and allotment letters, outbound voice calling.

## 2. Working agreement with the user

- The user is **learning Go while watching it built**. Keep Go plain and idiomatic (standard library first), comment concepts ("Go lesson: ..."), keep files small, and explain after each phase.
- **Never `git commit` or `git push` unless the user explicitly asks.** (They did ask once, on 2026-11-02, for the history below.) Never push. Suggest commit boundaries instead.
- Prefix shell commands with `rtk` (their global CLAUDE.md). It passes through when there is no filter.
- Give a recommendation, not a menu. The user pushes back when something is misread (e.g. "3D render" meant a **furnished flat**, not a floor plan).
- Be honest about limits. Label assumptions. Do not fabricate numbers (one test count was misquoted earlier and corrected).
- Never print secrets. `backend/.env` holds real keys; inspect names or prefixes only.

## 3. Stack and decisions

| Layer | Choice |
|---|---|
| Backend | Go (module `leadflow`, Go 1.27 installed), `chi`, `pgx/v5`, `goose` migrations (embedded), `bcrypt`, `x/time/rate`. Plain SQL in `internal/store`, **not sqlc** (easier to read while learning). |
| Frontend | React 19 + Vite + TypeScript, React Router 7, plain CSS with design tokens, `three` installed (unused so far). Node 22. |
| Database | Neon Postgres. `DATABASE_URL` = direct (migrations, seeding, tests); `DATABASE_URL_POOLED` = pooled (running app). |
| AI | **Google Gemini** (the user supplied a Gemini key, not Claude). Plan: `google.golang.org/genai` with function calling, behind a small Go interface so tests can fake it. |
| Deploy (plan) | Docker: Go serves the API and the built React files, one service on Fly.io or Render, Neon for the DB. Not done. |

Env vars in `backend/.env` (names): `DATABASE_URL`, `DATABASE_URL_POOLED`, `GEMINI_API_KEY`, optional `PORT`, `APP_ENV`, `ALLOWED_ORIGINS`, `SEED_DEMO_PASSWORD`. Frontend build flag `VITE_SHOW_DEMO_LOGINS`.

## 4. Repository layout

```
backend/
  cmd/{server,migrate,seed}/main.go
  internal/
    auth/       passwords, session tokens, Authenticate/RequireAuth/RequireRole
    booking/    PURE rules + error values (min notice, reminder planning, slot validation, visit codes)
    leads/      PURE rules: Input validation, phone normalising, fuzzy name match, Merge of two records
    config/     env loading (+ ALLOWED_ORIGINS)
    httpapi/    router, helpers (CSRF origin check, rate limits, JSON), auth + booking + lead handlers
    models/     one struct per table (leads store uses models.Lead; booking store and seed still use local types)
    seed/       sample-data seeder
    store/      ALL SQL: users/sessions, tx helper (retry on deadlock), booking engine,
                leads.go (ingest + dedupe), leads_query.go (list/detail), leads_merge.go (merge/dismiss)
    testdb/     throw-away-schema helper for integration tests (fixture: project-1 and project-2)
  migrations/   00001_init.sql + embed.go
frontend/
  src/{api,auth,pages}, src/index.css (design tokens), public/images/rooms/*.jpg
README.md       repo map + layering rules for human readers
deployment.md   local run steps, env var names, demo logins (deployment section still TODO)
HANDOFF.md      this file
```

Convention per domain: rules in a pure package, SQL in `store/<domain>*.go`, HTTP in `httpapi/<domain>_handlers.go`, tests beside each file.

## 5. How to run

```
cd backend
go run ./cmd/migrate          # create tables (already applied to the Neon DB)
go run ./cmd/seed             # only on an empty DB; add -reset to wipe and reseed
go run ./cmd/server           # API on :8080
cd ../frontend && npm install && npm run dev    # UI on :5173 (proxies /api to :8080)
cd ../backend && go test ./... -count=1         # about a minute (Neon latency)
```

Demo logins (password **`Demo@2026!`**, throwaway): `head@companya.test` (sales head), `rohan@companya.test` (Project 1), `aisha@companya.test` (Project 2), `vikram@companya.test` (both).

## 6. What is done (Phases 0 to 3)

**Phase 3 (leads and dedupe), uncommitted, added 2026-11-03:**
- `leads.Input.Clean` validates and normalises every enquiry. `NormalizePhone` accepts Indian mobiles only and returns `+91XXXXXXXXXX`. `NamesSimilar` (case/order/punctuation-insensitive, first+last word, about 15% edit distance, Hindi/Gujarati safe) drives the fuzzy flag. `Merge` combines two records (consent from either counts, opt-out from either wins).
- `store.IngestLead`: one transaction. `INSERT ... ON CONFLICT (phone) DO NOTHING`; if the phone exists the enquiry becomes a `lead_touches` row on the existing lead (first source, name and owner are kept, missing details filled, consent recorded, opt-out never cleared). A new lead is compared with up to 5,000 recent leads of the same project and flagged `possible_duplicate_of` the OLDEST similar one. Race test: 20 concurrent enquiries with one phone give exactly 1 lead.
- `store.ListLeads` / `GetLead`: executives see only leads assigned to them (enforced in SQL; others return 404). Filters: source, project, status, assignee (`unassigned` or an id; head only), needs attention, search (name, code, phone; `%` and `_` escaped), limit (max 100) and offset. Row includes `firstResponseSeconds`.
- `store.MergeDuplicate` / `DismissDuplicate` / `AssignLead`. Merge moves touches, conversations and visits to the original, deletes the duplicate, repoints other flags, adds an audit touch, and refuses (409) if both leads have a booked visit.
- API (staff, signed in): `GET /api/staff/leads`, `POST /api/staff/leads`, `POST /api/staff/leads/import` (up to 200 rows, each reported separately), `GET /api/staff/leads/{id}`; head only: `POST /api/staff/leads/{id}/assign` `{executiveId}` and `POST /api/staff/leads/{id}/duplicate` `{action: "merge"|"dismiss"}`. An executive who adds a lead owns it; adding a phone that belongs to another executive returns `merged` with no lead details (no data leak).
- Not built on purpose: public buyer enquiry endpoint (arrives with the chat in Phase 4, where the buyer-identity design is decided), chat history on lead detail (Phase 4).

**Phases 0 to 2:**

Git history on `main` (13 commits, local only, **not pushed**): gitignore/env template, schema migrations, models, auth, store base, booking rules, booking engine + tests, HTTP API, seed, room photos, frontend scaffold, frontend login, deployment.md.

- **Schema (12 tables)**, applied to Neon. The DB itself blocks double booking: unique index (one live visit per slot), unique index (one booked visit per lead), and a GiST exclusion constraint (one executive cannot own overlapping slots).
- **Booking engine** (`store/booking.go`): FreeSlots, BookSlot, RescheduleVisit (keeps the visit code, atomic), CancelVisit, MarkOutcome, CreateSlot, CancelSlot. Claiming a slot is one atomic `UPDATE ... WHERE status='open'` inside a transaction. The race test (50 buyers, 1 slot) passes: exactly one wins.
- **API**: `GET /api/healthz`; `POST /api/auth/{login,logout}`, `GET /api/auth/me`; public `GET /api/slots?project=&from=&to=`, `GET /api/visits/{code}`, `POST /api/visits/{code}/{cancel,reschedule}`; staff `POST /api/staff/visits`, `POST /api/staff/visits/{id}/{cancel,outcome}`; head-only `POST /api/staff/slots`, `DELETE /api/staff/slots/{id}`. Login rate-limited, public endpoints rate-limited, CSRF origin check, strict JSON decoding.
- **Frontend**: login screen (living-room photo, demo quick-fill), auth provider, route guard with role support, staff shell and a **placeholder** staff home. Verified in a real browser.
- **Seed data**: 163 leads (the 3 from the brief exact), 18 visits (12 past, 6 upcoming), 145 slots, 5 flagged possible duplicates, 174 lead touches, 18-entry fact sheet, spend at ₹1,000 per lead.
- **Tests**: 63 top-level tests (counted from `go test -v` on 2026-11-03, after Phase 3; subtests extra), all passing, none skipped. The store package takes about 100 s and httpapi about 40 s because of Neon latency. Unit tests (auth, booking rules, CSRF, validation) plus integration tests against a throw-away Neon schema.

## 7. What is next (remaining plan)

| Phase | Work |
|---|---|
| ~~3 Leads and dedupe~~ | **Done** (see section 6). |
| **4 AI agent** | Gemini function-calling loop. Tools: `get_free_slots`, `book_slot` (calls the existing engine), `save_profile` (budget, BHK, timeline), `request_human`, `flag_chat`. System prompt built from `fact_sheet_entries`. **Server-side guardrails**: block/rewrite any reply that quotes a price, discount or guarantee; reject tool args not from the fact sheet/DB; missing fact leads to "a person will confirm" plus handoff. English/Hindi/Gujarati. Scripted fallback if Gemini fails or rate-limits (lead is always saved first). Handoff summary to the named executive. Persist `conversations`/`messages`; set `leads.first_response_at`. A small **eval set (about 25 prompts)**: price traps, missing facts, injection attempts, the three languages. Buyer session: anonymous name + phone + **consent checkbox**; decide how the buyer proves identity to the booking tool. |
| **5 Buyer UI** | Chat screen (consent gate, language switch, streaming reply over SSE, quick replies, inline slot cards, confirmation with map pin, "talk to a person" always visible, RERA footer) and the `/visit/:code` page (view, reschedule, cancel, opt-out). Loading/empty/error states, mobile first. |
| **6 Staff UI** | Lead list (cards on mobile), lead detail (chat history, AI summary, visit panel, mark visited/no-show), dashboard (median first response, unreached in 48h, lead-to-visit, show-up rate, **cost per visit by source**), admin editor (fact sheet + slots), flagged-chat review queue, reminder log. Replace `StaffHome` placeholder. |
| **7 Reminders and compliance** | Background goroutine worker: select due `reminders` with `FOR UPDATE SKIP LOCKED`, render message (**must include the project RERA number**), mock messenger writes to the table, honour opt-out/consent. Opt-out ("STOP") handling. |
| **8 Hardening and deploy** | Dockerfile (Go serves built React), deploy, set `APP_ENV=production` (secure cookies), tidy empty/error states, accessibility pass, access-control tests for new routes, security review. |
| **9 Submission** | TDD (PDF/Word, **GitHub URL first**), finish `deployment.md`, clean history, zip (under 25 MB). |

### Optional extra layer: public browse + furnished 3D tour (do after the core works)
The user asked for a scrollable listings page with sort/filter and, on click, a **furnished 3D render of the flat** (not a floor plan) plus scrolling photos. Agreed approach: Three.js scene per **layout** (2 BHK, 3 BHK, a few scenes total) built from procedural walls plus CC0 GLB furniture (Poly Haven/Kenney), compressed (meshopt, KTX2), HDRI lighting, baked AO; dollhouse view plus click-a-room walk-in; render on demand, cap pixel ratio, lazy-load the chunk, pause off-screen, dispose on exit, static photo fallback on weak devices. Photos are Codex-generated. **Design canvas (private, Claude Design):** https://claude.ai/artifact/R6D4LBHX1jDPvVA48RbxJu (listings + residence page, desktop and phone, placeholders for the 3D). Scope warning already given to the user: this is extra depth, roughly 2 days, and must not starve the graded core.

## 8. Gotchas learned the hard way

- **Neon ignores `search_path` as a connection startup parameter** (silently). Use `SET search_path` in `AfterConnect`. `internal/testdb` does this and has a guard that refuses to run unless the tables really are in the temporary schema. Never run tests against `public`.
- goose's version table in `public` made a test migration think nothing was pending. `testdb` sets the table name to `<schema>.goose_db_version`.
- Vite dev origin (`localhost:5173`) differs from the API host, so the CSRF origin check needs `ALLOWED_ORIGINS` (defaults to `http://localhost:5173` when not production). Real login in the browser failed until this was added.
- Neon free DBs sleep; `store.Connect` retries with backoff.
- `rtk` prints a harmless "No hook installed" warning on every call.
- On Windows PowerShell the working tree path has a space: `E:\Sudhanshu\Projects\ISOCode Assesment`. Quote it.
- Codex CLI can generate images: `codex exec --skip-git-repo-check --sandbox workspace-write -C <dir> "<prompt>"` (about 2 minutes per image, ~3 MB PNG; resize to JPEG before use). It uses the user's OpenAI account.

## 9. Assumptions to label in the TDD

- Min booking notice 2 hours; slots are 1 hour; reminders at 1 day and 2 hours before; a reminder already past its time is skipped.
- A third role, **sales head**, with admin rights (the brief implies it; core build lists only buyer and executive).
- Buyers are anonymous (name, phone, consent); no buyer login.
- Per-source spend seeded at ₹1,000 per lead (the brief's cost per lead); the ₹30 lakh/month split by source is not given.
- Project names are "Project 1/2"; RERA numbers are placeholders (`RERA-PLACEHOLDER-P1/P2`); location pin is a placeholder near central Ahmedabad.
- Fact-sheet details (possession "December 2027", amenities, parking, areas) are invented sample content; payment plans, loans and discounts are deliberately absent so the assistant must hand off.
- All names and phone numbers (`+9190000...`) are invented. Listing units, areas and prices in the design canvas are invented sample data, labelled "indicative".
- Fact sheet is stored in English; the model translates answers into Hindi/Gujarati.
- Phone numbers: Indian mobiles only (ten digits starting 6 to 9; `+91`, `0` and `0091` prefixes accepted). Overseas numbers are a later extension.
- Fuzzy duplicate rule: same project, and names equal ignoring case, order and punctuation, or matching first and last word, or within about 15% edit distance. It only flags; a human (the head) merges or dismisses. The comparison scans up to 5,000 recent leads of the project (fine at about 3,000 enquiries a month; `pg_trgm` is the next step if volume grows).
- Staff who enter a lead attest that the person consented. A repeat enquiry can add consent but never clears an opt-out (opt-out stays sticky).
- Executives see only leads assigned to them; the head assigns unassigned leads. "Needs attention" means an open lead (new or engaged) with no first response yet, or flagged as a possible duplicate.
- The sample lead LD-58210's visit (2026-11-01 11:00) is already in the past relative to today (2026-11-02), so it is seeded as booked and awaiting an outcome.

## 10. Known loose ends

- `StaffHome` is a placeholder.
- `internal/models` is wired into the leads store only; the booking store and the seeder still use local types. `auth.Role*` now aliases `models.Role*`.
- The Vite template `frontend/README.md` and `public/icons.svg` were removed (staged deletions, uncommitted).
- `deployment.md` "How it is deployed" is a stub.
- No frontend tests yet. No reminder sender yet (rows are created, nothing sends them).
- **Uncommitted (the user commits).** Suggested commits: (1) `feat(leads): add lead rules package` (`internal/leads/`); (2) `feat(store): add lead ingest, list and duplicate merge` (`store/leads*.go`, `testdb` fixture); (3) `feat(api): add lead endpoints` (`httpapi/lead_handlers*.go`, `router.go`); (4) `chore: tidy repo layout` (`auth.go` role aliases, `README.md`, removed Vite leftovers, `HANDOFF.md`).
- The Codex test images and design canvas files live in scratch folders, not the repo.
