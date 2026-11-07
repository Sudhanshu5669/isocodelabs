# Session handoff

Read this first in a new session. It is written so someone with no memory of the earlier conversation can carry on. Last updated 2026-11-04 (evening), after the staff UI (Phase 6) was built. Buyer UI (Phase 5) was built earlier the same day. A repo map for human readers is in `README.md`.

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
| Database | Supabase Postgres (moved from Neon on 2026-11-03). `DATABASE_URL` = session pooler, port 5432 (migrations, seeding, tests); `DATABASE_URL_POOLED` = transaction pooler, port 6543 (running app; `store.Connect` switches pgx to the simple protocol on port 6543 because that pooler cannot keep prepared statements). Supabase Auth and `@supabase/server` are **not** used: the Go backend keeps its own session auth. |
| AI | Provider chosen by config: `AI_PROVIDER=gemini` (default, the user's key) or `openai`; `AI_MODEL` picks the model. All in `internal/ai/` behind the `ai.ChatCompletion` interface; plain REST calls, no SDKs. |
| Deploy (plan) | Docker: Go serves the API and the built React files, one service on Fly.io or Render, Supabase for the DB. Not done. |

Env vars in `backend/.env` (names): `DATABASE_URL`, `DATABASE_URL_POOLED`, `AI_PROVIDER` (gemini|openai|offline), `AI_MODEL`, `GEMINI_API_KEY`, `OPENAI_API_KEY` (not yet in the local .env), optional `OPENAI_BASE_URL`, `APP_ENV` (set to development locally; unset = production), `TRUSTED_PROXY_HEADER`, `PORT`, `ALLOWED_ORIGINS`, `SEED_DEMO_PASSWORD` (required in production). Test flags: `LEADFLOW_REQUIRE_DB=1`, `LEADFLOW_EVAL=1`, `EVAL_RPM`. Frontend build flag `VITE_SHOW_DEMO_LOGINS`.

## 4. Repository layout

```
backend/
  cmd/{server,migrate,seed}/main.go
  internal/
    ai/         EVERYTHING AI: completion.go (ChatCompletion interface), providers/ (Gemini, OpenAI,
                Offline + New(cfg)), assistant/ (agent loop, tools, prompt, guardrails, scripts, evals)
    auth/       passwords, session tokens, Authenticate/RequireAuth/RequireRole
    booking/    PURE rules + error values (min notice, reminder planning, slot validation, visit codes)
    leads/      PURE rules: Input validation, phone normalising, fuzzy name match, Merge of two records
    config/     env loading (+ ALLOWED_ORIGINS)
    httpapi/    router, helpers (CSRF origin check, rate limits, JSON), auth + booking + lead handlers
    models/     one struct per table (leads store uses models.Lead; booking store and seed still use local types)
    seed/       sample-data seeder
    store/      ALL SQL: users/sessions, tx helper (retry on deadlock), booking engine,
                leads.go (ingest + dedupe), leads_query.go (list/detail), leads_merge.go (merge/dismiss)
  migrations/   00001_init.sql + embed.go
  tests/        EVERY test, mirroring internal/ (tests/ai/assistant, tests/ai/providers, tests/auth,
                tests/booking, tests/httpapi, tests/leads, tests/store) + tests/testdb (throw-away-schema
                helper; fixture project-1 and project-2). Black-box packages (`<pkg>_test`): they use only
                exported names, so the few helpers tests need were exported (httpapi.ClientIP, NewIPLimiter,
                LoginRequest.Validate, SameOrigin, ChatState/ChatReply/ChatCookie, MaxTrackedKeys;
                assistant.MaxSteps; leads.EditDistance; providers.Encode*/Decode*). Evals:
                tests/ai/assistant/testdata/evals.json.
frontend/
  src/{api,auth,pages}, src/index.css (design tokens), public/images/rooms/*.jpg
README.md       repo map + layering rules for human readers
deployment.md   local run steps, env var names, demo logins (deployment section still TODO)
HANDOFF.md      this file
```

Convention per domain: rules in a pure package, SQL in `store/<domain>*.go`, HTTP in `httpapi/<domain>_handlers.go`, tests in `tests/<package>/` (the user asked for tests in a separate folder, not beside the code).

## 5. How to run

```
cd backend
go run ./cmd/migrate          # create tables (must be run once on the new Supabase DB)
go run ./cmd/seed             # only on an empty DB; add -reset to wipe and reseed
go run ./cmd/server           # API on :8080
cd ../frontend && npm install && npm run dev    # UI on :5173 (proxies /api to :8080)
cd ../backend && go test ./... -count=1 -timeout 30m   # about 11 minutes on Supabase (Sydney region latency)
```

Demo logins (password **`Demo@2026!`**, throwaway): `head@companya.test` (sales head), `rohan@companya.test` (Project 1), `aisha@companya.test` (Project 2), `vikram@companya.test` (both).

## 6. What is done (Phases 0 to 6, plus the backend of 7 and 8)

**Staff UI (Phase 6), uncommitted, added 2026-11-04.** All in `frontend/src/staff/` (+ `staff.css`), routes in `App.tsx`, menu in `pages/StaffShell.tsx` (head sees Dashboard, Review, Settings; the routes are also wrapped in `RequireAuth roles={['head']}`). `StaffHome` placeholder deleted.
- `LeadsPage` (`/staff`): filters in the URL (search with 300 ms debounce, status, source, project, assignee for the head, needs attention), pagination, cards on phones, duplicate/opt-out chips, first-reply time in amber when over 5 min or missing. `AddLeadForm` (consent box required; a known phone joins the existing lead).
- `LeadDetailPage` (`/staff/leads/:id`), the handoff screen: duplicate banner (head: merge / different people), chat transcripts with the handoff summary and staff notes (`ChatTranscript`), history (touches), buyer details with assign select (head), `VisitPanel` (mark came / did not come once started, cancel, book from free slots via `SlotPicker`), mark lost / reopen, record opt-out. On phones the buyer and visit panels come first.
- `VisitsPage` (diary, next/last two weeks, outcome buttons), `RemindersPage` (log with skip reasons and message text), `ReviewPage` (flagged chats, read inline, mark reviewed), `DashboardPage` (KPI tiles with brief targets, cost per visit by source bars, leads per day columns with hover tooltip + table view, by-source table; periods are month-aligned because spend counts whole months), `SettingsPage` tabs: `settings/FactsEditor`, `SlotsManager` (open several one-hour slots for a day), `SpendEditor` (6 months x 4 paid sources), `TeamManager` (add, switch off/on, new password).
- Shared: `staff/shared.tsx` (`ConfirmButton` asks in place instead of `window.confirm`, `useProjects`, `useTeam`, `Detail`, `budgetText`), `staff/Charts.tsx` (hand-made SVG/HTML charts, chart colour token `--chart-1: #1f8a6e` checked with the dataviz palette validator).
- Changed shared code: `lib/useApi` now drops data from an old path unless `{ keepPrevious: true }` (lead list and dashboard keep it, faded); `AuthProvider` retries `/auth/me` twice and shows "could not check your sign-in" with a retry instead of treating a 500 as signed out (`checkError`, `recheck` on the auth context).
- Verified with a headless Chrome script (playwright-core in the scratchpad, not in the repo): every staff screen as head and executive, desktop and 390 px phone; write flows passed through the UI and were undone (fact add/remove, slot open/remove, visit book/cancel, search). Executive sees only own 28 leads; executive visiting `/staff/dashboard` is redirected.


**Backend for Phases 5 to 8, uncommitted, added 2026-11-04** (the user asked to "build the whole backend"):
- Buyer: `GET /api/projects`, `GET /api/projects/{id}` (with public fact sheet), `POST /api/visits/{code}/opt-out`; `VisitDetail` has `optedOut`.
- Reminders (Phase 7): new package `internal/reminders` (`message.go` EN/HI/GU text with RERA number + "reply STOP", `decide.go` send-time checks, `messenger.go` Messenger interface + `LogMessenger` mock, `worker.go` 30 s loop started from `cmd/server/main.go`, stopped on shutdown). `store/reminders.go`: `ProcessDueReminders` claims ONE reminder per transaction with `FOR UPDATE SKIP LOCKED`, deliberately `runTx` (no retry, so no double send); `ReminderLog`; opt-out by visit code / phone / staff (skips queued reminders, adds a touch). STOP webhook `POST /api/webhooks/inbound` (`X-Webhook-Secret` = `INBOUND_WEBHOOK_SECRET`, constant-time compare; 404 when unset). Seed reminders now use `reminders.Render`.
- Staff (Phase 6 APIs): `GET /api/staff/dashboard` (head; cohort of leads arriving in the range; median first response, % within 5 min, unreached in 48 h, lead-to-visit = visited, show-up = visited/(visited+no-show), spend/cost per lead/cost per visit by source; whole months of spend; cost empty with a project filter; daily series) in `store/dashboard.go`; fact editor `GET /api/staff/projects/{id}/facts`, head `PUT/DELETE .../facts/{topic}` (live for the very next chat message); spend `GET/PUT /api/staff/spend` (head); `GET /api/staff/visits`, `/slots`, `/reminders` (executives see own, in SQL); `POST /api/staff/leads/{id}/status` (lost/engaged only; refused while a visit is booked) and `/opt-out`; users (head): `GET/POST /api/staff/users`, `POST /users/{id}/active` (switch-off deletes sessions, cancels their open future slots, reports visits/leads to reassign; cannot switch off yourself), `POST /users/{id}/password` (signs out everywhere). Handoff no longer picks a switched-off assignee.
- Deploy (Phase 8): `httpapi/static.go` serves `STATIC_DIR` (SPA fallback to index.html, immutable cache for `/assets/`, strict CSP on pages, GET/HEAD only). Root `Dockerfile` (node build -> go build -> alpine, non-root, healthcheck, runs `migrate` then `server`) and `.dockerignore`. New env: `STATIC_DIR`, `PUBLIC_URL`, `INBOUND_WEBHOOK_SECRET`, `REMINDERS=off`.
- Decided NOT to stream chat replies over SSE: the guardrail must check the whole reply before the buyer sees any of it.
- Verified: real server on the seeded Supabase DB served the app + API (dashboard last 30 days: 93 leads, median first response 105 s, lead-to-visit 6.5%, show-up 86%, cost per visit Rs 16,500).


**Phase 4 (AI agent), uncommitted, added 2026-11-03:**
- Folders: everything AI lives in `internal/ai/` (restructured at the user's request): `ai/completion.go` (the `ChatCompletion` interface: `Complete`, `Provider`, `Model`, plus the shared message types), `ai/providers/` (one struct per provider: `Gemini`, `OpenAI`, `Offline`; `providers.New(cfg)` picks one from `AI_PROVIDER`; the ONLY code that calls a model API, plain `net/http`; Google's Go SDK was dropped because its dependency tree is huge; OpenAI is unit-tested on its JSON but not yet called for real), `ai/assistant/` (pure: agent loop `agent.go`, 5 tools `tools.go`, system prompt from the fact sheet `prompt.go`, guardrails `guard.go`, fixed EN/HI/GU texts `scripts.go`, handoff summary `summary.go`), `store/chat.go` + `chat_staff.go`, `httpapi/chat_handlers.go` + `chat_actions.go` (the bridge from tools to the store). Migrations `00002_chat.sql`, `00003_chat_isolation.sql` (applied to Supabase).
- Buyer API (no login, `chat` cookie on `/api/chat` only): `POST /api/chat/start` (name, phone, project, language, consent required; lead saved via IngestLead as source `website`; greeting sent instantly without the model, sets `first_response_at`), `GET /api/chat`, `POST /api/chat/messages`, `POST /api/chat/book` (slot card, no model), `POST /api/chat/handoff` ("talk to a person"). Staff: lead detail now has `chats` (with staff-only system notes) and `timeline`; head-only `GET /api/staff/chats/flagged`, `POST /api/staff/chats/{id}/reviewed`.
- Guardrails in code, not just the prompt: every money figure in a reply must appear in the fact sheet (any digits, EN/HI/GU units); discount/guarantee words blocked; blocked reply replaced by a fixed message, chat flagged, original kept as a staff note. Injection-looking buyer messages flag the chat. `book_slot` only accepts ids returned by `get_free_slots` in the same turn, and the store re-checks the project. Model failure, timeout or endless tool calls -> fixed reply in the buyer's language + handoff to a named executive + slot cards.
- Handoff picks the lead's executive, else the one who shows the project with the fewest open leads. Summary (facts from the DB + a model-written gist) is written in the background (`Server.background` WaitGroup, waited on at shutdown and in tests).
- Eval set: `tests/ai/assistant/testdata/evals.json` (26 cases: facts, missing facts, price traps, injection, booking/handoff/profile flows, Hindi/Gujarati). Run: `LEADFLOW_EVAL=1 go test ./tests/ai/assistant -run TestEvals -v` (uses whatever `AI_PROVIDER`/`AI_MODEL` say) (paced to `EVAL_RPM`, default 4/min). **Only partly measured**: 9 cases got real answers before the free-tier daily quota ran out; 8 passed (all 5 fact cases, 3 of 4 missing-fact cases), `missing-schools` answered without handing off. The rest still need a run.

**Security review fixes (two reviewers, 2026-11-03):**
- Chat identity: a typed phone proves nothing. A chat sees only the name typed in it and a visit booked in it (`conversations.buyer_name`, `visit_id`); if the phone already had a lead (`new_lead = false`) the chat cannot book (a person confirms by phone) or overwrite recorded details (only fills blanks). Chat tokens expire on the server (`token_expires_at`, 30 days). Real fix later: OTP over SMS/WhatsApp.
- Rate limits: `middleware.RealIP` removed (it trusted client-sent X-Forwarded-For). `ClientIP` uses the connection address, or only the header named in `TRUSTED_PROXY_HEADER`. Limiter no longer wipes itself at 10,000 keys (evicts idle buckets only). New per-account limit: 5 failed logins / 15 min per email.
- `APP_ENV` unset = production; anything but development/production is an error; cookies Secure unless development. HSTS when secure; `Cache-Control: no-store` + deny-all CSP on `/api`.
- Login: DB errors are 500 + logged (not "wrong password"); disabled accounts run bcrypt before refusal (same timing); display-name emails rejected; clearer 72-byte message. Middleware: DB errors are 500, not "signed out". Logout: if the session cannot be deleted it returns 500 and keeps the cookie; frontend keeps the user signed in and shows an alert. Frontend: any 401 (except login) clears the user.
- Seed: production refuses `-reset` and the built-in demo password, and never prints the password. Tests: loud SKIPPING banner without a DB; `LEADFLOW_REQUIRE_DB=1` makes it fatal.
- Not done (noted by reviewers, judged later work): user management endpoints (create/disable/reset password), bcrypt cost 12 (needs reseeding so the dummy hash cost matches), a CSP for the HTML page (Phase 8, when Go serves the UI), `models.User` vs `auth.User` duplication (kept: `auth.User` is the hash-free view).

**Phase 3 (leads and dedupe), uncommitted, added 2026-11-03:**
- `leads.Input.Clean` validates and normalises every enquiry. `NormalizePhone` accepts Indian mobiles only and returns `+91XXXXXXXXXX`. `NamesSimilar` (case/order/punctuation-insensitive, first+last word, about 15% edit distance, Hindi/Gujarati safe) drives the fuzzy flag. `Merge` combines two records (consent from either counts, opt-out from either wins).
- `store.IngestLead`: one transaction. `INSERT ... ON CONFLICT (phone) DO NOTHING`; if the phone exists the enquiry becomes a `lead_touches` row on the existing lead (first source, name and owner are kept, missing details filled, consent recorded, opt-out never cleared). A new lead is compared with up to 5,000 recent leads of the same project and flagged `possible_duplicate_of` the OLDEST similar one. Race test: 20 concurrent enquiries with one phone give exactly 1 lead.
- `store.ListLeads` / `GetLead`: executives see only leads assigned to them (enforced in SQL; others return 404). Filters: source, project, status, assignee (`unassigned` or an id; head only), needs attention, search (name, code, phone; `%` and `_` escaped), limit (max 100) and offset. Row includes `firstResponseSeconds`.
- `store.MergeDuplicate` / `DismissDuplicate` / `AssignLead`. Merge moves touches, conversations and visits to the original, deletes the duplicate, repoints other flags, adds an audit touch, and refuses (409) if both leads have a booked visit.
- API (staff, signed in): `GET /api/staff/leads`, `POST /api/staff/leads`, `POST /api/staff/leads/import` (up to 200 rows, each reported separately), `GET /api/staff/leads/{id}`; head only: `POST /api/staff/leads/{id}/assign` `{executiveId}` and `POST /api/staff/leads/{id}/duplicate` `{action: "merge"|"dismiss"}`. An executive who adds a lead owns it; adding a phone that belongs to another executive returns `merged` with no lead details (no data leak).
- Not built on purpose: public buyer enquiry endpoint (arrives with the chat in Phase 4, where the buyer-identity design is decided), chat history on lead detail (Phase 4).

**Phases 0 to 2:**

Git history on `main` (13 commits, local only, **not pushed**): gitignore/env template, schema migrations, models, auth, store base, booking rules, booking engine + tests, HTTP API, seed, room photos, frontend scaffold, frontend login, deployment.md.

- **Schema (12 tables)**, applied to Neon at the time; re-apply on Supabase. The DB itself blocks double booking: unique index (one live visit per slot), unique index (one booked visit per lead), and a GiST exclusion constraint (one executive cannot own overlapping slots).
- **Booking engine** (`store/booking.go`): FreeSlots, BookSlot, RescheduleVisit (keeps the visit code, atomic), CancelVisit, MarkOutcome, CreateSlot, CancelSlot. Claiming a slot is one atomic `UPDATE ... WHERE status='open'` inside a transaction. The race test (50 buyers, 1 slot) passes: exactly one wins.
- **API**: `GET /api/healthz`; `POST /api/auth/{login,logout}`, `GET /api/auth/me`; public `GET /api/slots?project=&from=&to=`, `GET /api/visits/{code}`, `POST /api/visits/{code}/{cancel,reschedule}`; staff `POST /api/staff/visits`, `POST /api/staff/visits/{id}/{cancel,outcome}`; head-only `POST /api/staff/slots`, `DELETE /api/staff/slots/{id}`. Login rate-limited, public endpoints rate-limited, CSRF origin check, strict JSON decoding.
- **Frontend**: login screen (living-room photo, demo quick-fill), auth provider, route guard with role support, staff shell and a **placeholder** staff home. Verified in a real browser.
- **Seed data**: 163 leads (the 3 from the brief exact), 18 visits (12 past, 6 upcoming), 145 slots, 5 flagged possible duplicates, 174 lead touches, 18-entry fact sheet, spend at ₹1,000 per lead.
- **Tests (after Phase 4, 2026-11-03)**: 104 top-level tests, all passing with `LEADFLOW_REQUIRE_DB=1` (store ~420 s, httpapi ~420 s on Supabase). Earlier note: 63 top-level tests (counted from `go test -v` on 2026-11-03, after Phase 3; subtests extra), all passing, none skipped. On Supabase (2026-11-03) the store package takes about 470 s and httpapi about 210 s because of remote DB latency (Sydney region). Unit tests (auth, booking rules, CSRF, validation) plus integration tests against a throw-away schema in the real database.

## 7. What is next (remaining plan)

| Phase | Work |
|---|---|
| ~~3 Leads and dedupe~~ | **Done** (see section 6). |
| ~~4 AI agent~~ | **Done** (see section 6), except a complete eval run. Original plan: |
| (4, plan as written) | Gemini function-calling loop. Tools: `get_free_slots`, `book_slot` (calls the existing engine), `save_profile` (budget, BHK, timeline), `request_human`, `flag_chat`. System prompt built from `fact_sheet_entries`. **Server-side guardrails**: block/rewrite any reply that quotes a price, discount or guarantee; reject tool args not from the fact sheet/DB; missing fact leads to "a person will confirm" plus handoff. English/Hindi/Gujarati. Scripted fallback if Gemini fails or rate-limits (lead is always saved first). Handoff summary to the named executive. Persist `conversations`/`messages`; set `leads.first_response_at`. A small **eval set (about 25 prompts)**: price traps, missing facts, injection attempts, the three languages. Buyer session: anonymous name + phone + **consent checkbox**; decide how the buyer proves identity to the booking tool. |
| ~~5 Buyer UI~~ **Built** (not yet browser-tested end to end) | Chat screen (consent gate, language switch, streaming reply over SSE, quick replies, inline slot cards, confirmation with map pin, "talk to a person" always visible, RERA footer) and the `/visit/:code` page (view, reschedule, cancel, opt-out). Loading/empty/error states, mobile first. |
| ~~6 Staff UI~~ **Done** (see section 6). Plan as written: | Lead list (cards on mobile), lead detail (chat history, AI summary, visit panel, mark visited/no-show), dashboard (median first response, unreached in 48h, lead-to-visit, show-up rate, **cost per visit by source**), admin editor (fact sheet + slots), flagged-chat review queue, reminder log. Replace `StaffHome` placeholder. |
| ~~7 Reminders and compliance~~ **Done** (backend only phase) | Background goroutine worker: select due `reminders` with `FOR UPDATE SKIP LOCKED`, render message (**must include the project RERA number**), mock messenger writes to the table, honour opt-out/consent. Opt-out ("STOP") handling. |
| **8 Hardening and deploy** (Dockerfile + static serving done) | Deploy, set `APP_ENV=production` (secure cookies), tidy empty/error states, accessibility pass, access-control tests for new routes, security review. |
| **9 Submission** | TDD (PDF/Word, **GitHub URL first**), finish `deployment.md`, clean history, zip (under 25 MB). |

### Optional extra layer: public browse + furnished 3D tour (do after the core works)
The user asked for a scrollable listings page with sort/filter and, on click, a **furnished 3D render of the flat** (not a floor plan) plus scrolling photos. Agreed approach: Three.js scene per **layout** (2 BHK, 3 BHK, a few scenes total) built from procedural walls plus CC0 GLB furniture (Poly Haven/Kenney), compressed (meshopt, KTX2), HDRI lighting, baked AO; dollhouse view plus click-a-room walk-in; render on demand, cap pixel ratio, lazy-load the chunk, pause off-screen, dispose on exit, static photo fallback on weak devices. Photos are Codex-generated. **Design canvas (private, Claude Design):** https://claude.ai/artifact/R6D4LBHX1jDPvVA48RbxJu (listings + residence page, desktop and phone, placeholders for the 3D). Scope warning already given to the user: this is extra depth, roughly 2 days, and must not starve the graded core.

## 8. Gotchas learned the hard way

- **Poolers ignore `search_path` as a connection startup parameter** (seen silently on Neon). Use `SET search_path` in `AfterConnect`. `tests/testdb` does this and has a guard that refuses to run unless the tables really are in the temporary schema. Never run tests against `public`.
- goose's version table in `public` made a test migration think nothing was pending. `testdb` sets the table name to `<schema>.goose_db_version`.
- Vite dev origin (`localhost:5173`) differs from the API host, so the CSRF origin check needs `ALLOWED_ORIGINS` (defaults to `http://localhost:5173` when not production). Real login in the browser failed until this was added.
- `store.Connect` retries with backoff (free Supabase projects pause after a week idle; restore them from the dashboard).
- **Supabase session pooler allows only 15 clients in total.** `tests/testdb` keeps its pools tiny (admin 1, test pool 5, goose 1 and closed after migrating); raising them makes concurrency tests fail with `EMAXCONNSESSION`.
- Supabase direct host (`db.<ref>.supabase.co`) is IPv6 only on the free plan, so use the pooler URLs.
- **Gemini free tier is tiny**: gemini-2.5-flash allows 5 requests/minute and **20 per day** on this key; one buyer message can take 2 to 4 calls. Enough for a short demo only. The default is now `gemini-3.5-flash-lite` (bigger free allowance; called tools correctly in a probe on 2026-11-03; `gemini-3.1-flash-lite` also did). Switch model with `AI_MODEL`, or company with `AI_PROVIDER=openai` (`gpt-5-nano`, roughly $0.001 per buyer message). Production needs billing on whichever key is used.
- `APP_ENV` unset now means **production** (secure cookies, no dev origin). Local `.env` has `APP_ENV=development`.
- Shell heredocs with Python that contain `''` broke once in this Bash tool; writing the script to the scratchpad and running it works.
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

- **Supabase latency is the main risk for the demo.** One DB round trip is about 0.4 s from here (project is in the Sydney region). Endpoints alone take 1.5 to 3.5 s, but when the browser fires 4 to 6 at once some exceed the server's 30 s timeout (seen on list leads, staff visits, list users). The session lookup is a plain SELECT, so it is not lock contention. Fix to try first: move the Supabase project to Mumbai (ap-south-1) or deploy the app in the same region as the DB; then compare the session pooler (5432) with the transaction pooler (6543) under concurrency.
- The review queue is empty in the seed data (no flagged chats); seed one or two for the demo.
- Seed data has a lead touch dated in the future (LD-58234, "5 Nov, 9:58 pm" seen on 4 Nov): seed times are probably offset from the wrong base.
- ESLint is not installed in `frontend` (`npx eslint` tries to download it).
- `internal/models` is wired into the leads store only; the booking store and the seeder still use local types. `auth.Role*` now aliases `models.Role*`.
- The Vite template `frontend/README.md` and `public/icons.svg` were removed (staged deletions, uncommitted).
- `deployment.md` "How it is deployed" is a stub.
- No frontend tests yet. No reminder sender yet (rows are created, nothing sends them).
- **Uncommitted (the user commits).** Suggested commits: (1) `feat(leads): add lead rules package` (`internal/leads/`); (2) `feat(store): add lead ingest, list and duplicate merge` (`store/leads*.go`, `testdb` fixture); (3) `feat(api): add lead endpoints` (`httpapi/lead_handlers*.go`, `router.go`); (4) `chore: tidy repo layout` (`auth.go` role aliases, `README.md`, removed Vite leftovers, `HANDOFF.md`).
- The Codex test images and design canvas files live in scratch folders, not the repo.

## 11. DPDP encryption of personal data (2026-11-07)

- `backend/internal/pii`: AES-256-GCM (random 12-byte nonce, the data's kind bound in as GCM additional data, key id in each ciphertext for rotation) and HMAC-SHA-256 blind indexes with a separate key.
- The "interceptor" is in the pgx driver: store code passes `pii.Seal(kind, v)` as an argument and `pii.Open(kind, &v)` as a scan target; pgx calls `BytesValue`/`TextValue`/`ScanBytes`, which pgx checks before its generic fallbacks, so plaintext never reaches Postgres in either the binary or the simple (transaction pooler) protocol. `tests/pii` proves this with pgx's type map.
- Encrypted columns (bytea): users.email/name, leads.name/phone, conversations.buyer_name/summary, messages.content, lead_touches.note, visits.notes, reminders.body. Blind indexes: users.email_bidx (unique, sign-in), leads.phone_bidx (unique, dedupe, STOP opt-out, search), leads.name_words bytea[] + GIN (whole-word name search). flag_reason stays plain (system text only).
- Behaviour changes: lead search matches code (partial), whole name words, or a full phone number; no partial name/phone matches. Sorting by staff name moved from SQL to Go.
- Migrations 00005 (add twins) → 00006 (Go: encrypt in batches) → 00007 (drop plaintext, rename, constraints). All reversible; `tests/migrations` round-trips them. Applied to the live Supabase DB on 2026-11-07.
- Keys: `PII_ENCRYPTION_KEYS` and `PII_INDEX_KEY` in `backend/.env` (generated locally, never printed). Required; back them up. Production needs them as platform secrets.
- Known flaky test: `TestSpoofedForwardedForDoesNotBypassLoginLimit` fails when the 11 logins take over 6 s (limiter refill) under parallel DB load; it passes alone.
