# Deployment and running guide

**Live:** https://isocodelabs.onrender.com

## Demo logins (for the reviewer)

These are throwaway accounts that exist only in the seeded sample database. All use the same demo password.

| Role | Email | What they can do |
|---|---|---|
| Sales head | `head@companya.test` | Everything: all leads, dashboard, fact-sheet and slot editor, flagged-chat review |
| Sales executive | `rohan@companya.test` | Own leads and visits (Project 1) |
| Sales executive | `aisha@companya.test` | Own leads and visits (Project 2) |
| Sales executive | `vikram@companya.test` | Own leads and visits (both projects) |

**Password for all four:** `Demo@2026!`

Buyers do not sign in. The public chat asks for a name, phone number and consent.

All names, phone numbers (`+9190000…`), RERA numbers and fact-sheet details in the sample data are invented placeholders.

A fifth account, `kunal@companya.test`, is **switched off** on purpose: signing in with it is refused, and the sales head sees his leads and visit waiting to be handed to someone else (Settings → Team).

## Test scenarios in the sample data

`go run ./cmd/seed -reset` creates about 190 leads: 163 random ones over the last 60 days (including the brief's LD-58210, LD-58211 and LD-58224 exactly as given) plus the hand-made cases below. Search a code in the lead list to open it. Times are relative to when the seed ran.

| Lead | What to check |
|---|---|
| LD-59001 Aarav Shah | English chat that booked a visit from a slot card; visit pass, reminders queued |
| LD-59002 Sneha Patel | Hindi chat; asked about home loans (not on the fact sheet), so handed to Aisha with a summary |
| LD-59003 Kiran Desai | Gujarati chat; asked about schools (missing fact), then "talk to a person" |
| LD-59004 Manish Rathod | **Review queue:** the model offered a discount, the guardrail blocked it (staff note shows the blocked text) |
| LD-59005 Rakesh Bhatt | **Review queue:** "ignore all previous instructions" injection attempt |
| LD-59006 Tejal Mehta | Flagged price quote that the head has already reviewed (on the lead, not in the queue) |
| LD-59007 Deepak Vora | Model unavailable: fixed reply, handoff, summary without a chat gist |
| LD-59008 Hemal Joshi | New 12 minutes ago, nobody has replied, not assigned (needs attention) |
| LD-59009 Nayan Chauhan | Not reached in over 48 hours (counts against the dashboard target) |
| LD-59010 Farhan Qureshi | Walk-in with no project or apartment type chosen yet |
| LD-59011 Gaurav Pandya | No consent recorded: must not be messaged |
| LD-59012 Pinal Soni | Opted out (replied STOP) but has a booked visit: reminders skipped |
| LD-59013 / LD-59014 Bhavesh Solanki | Possible duplicates, both with a booked visit: merge is refused until one visit is cancelled |
| LD-59015 / LD-59016 Nikita Parmar / Parmar Nikita | Possible duplicates (name order swapped): safe to merge or mark as different people |
| LD-59017 Sagar Thakkar | Visit cancelled by the buyer: slot open again, both reminders skipped |
| LD-59018 Ritu Amin | Visit moved to a later day by the buyer |
| LD-59019 Jay Vyas | Visit early tomorrow: day-before reminder sent, 2-hour reminder waiting |
| LD-59020 Mehul Gandhi | Past no-show; one reminder failed to send |
| LD-59021 Kavya Rana | Past visit, came; one reminder skipped as too late |
| LD-59022 Om Prajapati | Visit time has passed, outcome not recorded yet ("did the buyer come?") |
| LD-59023 Shreya Dave | Lost lead, reason in its history |
| LD-59030 to LD-59032 | Leads (one with an upcoming visit) of the switched-off executive Kunal Shah |
| LD-58210 Hardik Patel | From the brief: visit on 1 Nov 11:00 has passed and awaits an outcome |

Also in the data: random leads from every source in all five statuses, five more flagged duplicates, repeat enquiries merged by phone number, sent reminders in English, Hindi and Gujarati, open and booked slots for the next two weeks, and six months of spend per source (website spend for the current month is deliberately left blank). The 12 sample flats for the public catalogue are seeded too.

## Run it locally

Requirements: Go 1.22 or newer, Node 20 or newer, and a Postgres database (the project uses Supabase).

1. **Configure the backend.** Copy `backend/.env.example` to `backend/.env` and fill in the values (names listed below).
2. **Create the tables.**
   ```
   cd backend
   go run ./cmd/migrate
   ```
3. **Load the sample data** (only works on an empty database; add `-reset` to wipe and reseed).
   ```
   go run ./cmd/seed
   ```
4. **Start the API** (port 8080).
   ```
   go run ./cmd/server
   ```
5. **Start the frontend** in a second terminal (port 5173).
   ```
   cd frontend
   npm install
   npm run dev
   ```
6. Open http://localhost:5173/staff and sign in with a demo login above.

Run the backend tests with `go test ./... -timeout 30m` in `backend` (all tests live in `backend/tests/`; the database tests take about 11 minutes against a remote Supabase region, past Go's default 10-minute limit).

**One server for everything (as in production):** build the frontend with `npm run build` in `frontend`, then start the API with `STATIC_DIR=../frontend/dist go run ./cmd/server` and open http://localhost:8080.

**With Docker** (from the repo root):
```
docker build -t leadflow .
docker run -p 8080:8080 --env-file backend/.env -e APP_ENV=development leadflow
```
The container applies pending migrations, then starts the server, which serves the API and the web app on port 8080.

## Environment variables (names only)

| Name | Used for |
|---|---|
| `DATABASE_URL` | Supabase session-pooler connection (port 5432), used by migrations, seeding and tests |
| `DATABASE_URL_POOLED` | Supabase transaction-pooler connection (port 6543), used by the running app |
| `AI_PROVIDER` | Which company's model runs the chat: `gemini` (default), `openai`, or `offline` (fixed replies only). This one setting switches provider |
| `AI_MODEL` | Optional: the model, e.g. `gemini-3.5-flash-lite` or `gpt-6-luna`. Empty = that provider's default (`gemini-3.5-flash-lite` / `gpt-6-luna`) |
| `GEMINI_API_KEY` | Google Gemini key, needed when `AI_PROVIDER=gemini`. Free keys have small daily limits; the lite models allow the most |
| `OPENAI_API_KEY` | OpenAI key, needed when `AI_PROVIDER=openai` |
| `OPENAI_BASE_URL` | Optional: an OpenAI-compatible API instead of OpenAI (default `https://api.openai.com/v1`) |
| `OPENAI_REASONING_EFFORT` | Optional: OpenAI's `reasoning_effort`. Empty = `none`, which `gpt-6-luna` needs to use tools on the Chat Completions API (and keeps replies fast and cheap). Older `gpt-5`/`gpt-5-mini`/`gpt-5-nano` need `minimal`; `off` leaves it out for models without reasoning such as `gpt-4.1-mini` |
| `PORT` | HTTP port (default 8080) |
| `APP_ENV` | `development` or `production`. **Unset means production** (secure cookies, HSTS, no dev origins); any other value stops the server. Set `development` only on your own machine |
| `TRUSTED_PROXY_HEADER` | The one header your hosting platform's proxy sets to the real client IP, e.g. `Fly-Client-IP` (Fly.io) or `X-Forwarded-For` (Render, where the last entry is used). Leave empty when nothing sits in front of the server. Rate limits use it, so never set it to a header clients can send directly |
| `ALLOWED_ORIGINS` | Extra browser origins allowed to send POST requests (default `http://localhost:5173` in development, none in production) |
| `STATIC_DIR` | Folder with the built React app; when set, the server serves the whole site (the Docker image sets `/app/web`). Empty = API only |
| `PUBLIC_URL` | Optional: the site's public address, e.g. `https://visits.example.com`. Reminder messages link to `PUBLIC_URL/visit/<code>` |
| `INBOUND_WEBHOOK_SECRET` | Optional: shared secret the messaging provider sends in `X-Webhook-Secret` when a buyer replies (a "STOP" reply opts them out). Empty = the endpoint is off |
| `REMINDERS` | Set to `off` to stop the background reminder sender on this server (default on). Running it on several servers is safe |
| `SEED_DEMO_PASSWORD` | Staff password the seeder sets. Optional with `APP_ENV=development` (the demo password above is used); **required in production**, where the seeder also refuses `-reset` |
| `VITE_SHOW_DEMO_LOGINS` | Optional frontend build flag: `true` shows the demo-account buttons on the login page |
| `PII_ENCRYPTION_KEYS` | **Required.** AES-256-GCM key(s) for personal data (DPDP), as `id:base64`, e.g. `1:<openssl rand -base64 32>`. To rotate, add a new id (`1:<old>,2:<new>`): new data uses the highest id, old data still decrypts |
| `PII_INDEX_KEY` | **Required.** Separate 32-byte base64 key for the HMAC-SHA-256 blind indexes (sign-in by email, one lead per phone, STOP opt-out, lead search). Must differ from the encryption keys and cannot be rotated without re-indexing |

Never commit `backend/.env`. It is listed in `.gitignore`.

### Personal data protection (DPDP)

Names, phone numbers, emails, chat messages, AI summaries, staff notes and reminder texts are encrypted with AES-256-GCM in the database driver before they reach Postgres (`backend/internal/pii`), so the database, its backups and anyone with SQL access see only ciphertext. Lookups use HMAC-SHA-256 blind indexes instead of the values. Staff search therefore matches a lead code (partly), whole words of a name, or a complete phone number, but not part of a name or phone.

**Back up both keys somewhere other than the database.** Without them the data cannot be decrypted. Migrations 00005 to 00007 encrypt an existing plaintext database in place (they need the keys), and `go run ./cmd/migrate down` three times decrypts it back.

## How it is deployed

The `Dockerfile` at the repo root builds one image: the React app (Node 22), the Go binaries (`server`, `migrate`, `seed`), and a small Alpine runtime that runs as a non-root user. On start it runs `./migrate` and then `./server`. Any container host works (Fly.io, Render, Railway); the database stays on Supabase.

Set `APP_ENV=production` (the image's default), the database URLs, the AI variables, `PII_ENCRYPTION_KEYS` and `PII_INDEX_KEY` (as platform secrets; the server will not start without them), and `TRUSTED_PROXY_HEADER` for your host. The image has a health check on `/healthz`.

### Live deployment

**URL:** https://isocodelabs.onrender.com (buyer site at `/`, staff app at `/staff`, sign in with a demo login above).

It runs as one **Render web service** (free instance, Singapore region, the closest Render region to the Supabase database in Sydney):

- **Build:** Render builds the root `Dockerfile` straight from the GitHub repo and redeploys on every push to `main`. The frontend is built inside the image and served by the Go server from the same origin, so there is no separate frontend service, no CORS setup and `ALLOWED_ORIGINS` stays empty.
- **Configuration:** set in Render's Environment tab (there is no `.env` file in the repo or the image): `DATABASE_URL`, `DATABASE_URL_POOLED`, `AI_PROVIDER=openai`, `AI_MODEL=gpt-6-luna`, `OPENAI_API_KEY`, `PII_ENCRYPTION_KEYS`, `PII_INDEX_KEY`, plus `PORT=8080`, `TRUSTED_PROXY_HEADER=X-Forwarded-For` and `PUBLIC_URL=https://isocodelabs.onrender.com`. `APP_ENV` is left unset, which means production.
- **Health check:** Render checks `/healthz`, which also pings the database.
- **Staying awake:** free Render instances sleep after 15 minutes without traffic. A free [cron-job.org](https://cron-job.org) job calls `/healthz` every 10 minutes so the site stays awake for reviewers and the background reminder sender keeps running. One always-on service uses about 744 of the 750 free instance hours a month.
- **Database:** the existing Supabase project (Sydney), already migrated and seeded. The container still runs `./migrate` on every start, which does nothing when the schema is current.

Checked after deploying: health check, both apps, public API, demo sign-in (session cookie `HttpOnly; Secure; SameSite=Lax`), six staff requests in parallel all answering in under 1.5 s, cross-site POSTs refused with 403, and the security headers (CSP, HSTS, `X-Frame-Options: DENY`, `nosniff`).
