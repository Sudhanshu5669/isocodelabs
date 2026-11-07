# Leadflow: lead-to-visit assistant for Company A

An AI-assisted enquiry desk for a residential developer in Ahmedabad. It collects
leads from every source into one list, checks for duplicates, answers buyers from
an approved fact sheet, books site visits into real free slots (never double
booked), and hands serious buyers to a named salesperson.

- **How to run it, env var names and demo logins:** [`deployment.md`](deployment.md)
- **Where the project stands and what is next:** [`HANDOFF.md`](HANDOFF.md)

## Repository map

```
.
├── backend/                     Go API (module "leadflow")
│   ├── cmd/
│   │   ├── server/              starts the API (and, later, serves the built UI)
│   │   ├── migrate/             applies database migrations
│   │   └── seed/                fills an empty database with sample data
│   ├── migrations/              SQL schema, embedded into the binary
│   ├── tests/                   EVERY test, in folders that mirror internal/ (see "Tests" below)
│   └── internal/                application code only (no tests), one package per concern:
│       ├── config/              environment variables
│       ├── models/              one struct per database table, plus allowed values
│       ├── auth/                passwords, sessions, who-may-do-what middleware
│       ├── booking/             pure rules: notice period, reminder times, slot checks
│       ├── leads/               pure rules: phone normalising, name matching, merging
│       ├── reminders/           visit reminders: message text (EN/HI/GU, RERA no.), send-time
│       │                        checks, mock messenger, background worker, STOP detection
│       ├── ai/                  EVERYTHING to do with AI
│       │   ├── completion.go    ChatCompletion: the interface every provider implements
│       │   ├── providers/       one struct per provider (Gemini, OpenAI, Offline) + New(), which
│       │   │                    picks one from AI_PROVIDER; the ONLY code that calls a model API
│       │   └── assistant/       the sales assistant: agent loop, tools, prompt, guardrails, fixed replies
│       ├── pii/                 DPDP: AES-256-GCM encryption of personal data in the DB driver
│       │                        (Seal/Open hooks pgx calls) + HMAC-SHA-256 blind indexes for lookups
│       ├── store/               ALL SQL lives here (booking*, leads*, chat*, users)
│       ├── httpapi/             routes, handlers, request checks
│       └── seed/                the sample-data seeder
├── frontend/                    React + TypeScript (Vite)
│   └── src/
│       ├── api/                 the one place that talks to the backend
│       ├── auth/                sign-in state and route guards
│       ├── pages/               one file per screen (+ its CSS)
│       └── index.css            design tokens: colours, type, spacing
├── Dockerfile                   one image: builds the React app and the Go server, serves both
├── deployment.md                run, deploy, env var names, demo logins
└── HANDOFF.md                   session notes for whoever continues the work
```

## How the backend is organised

Code flows one way: **httpapi → store → database**, with the rule packages
(`booking`, `leads`, `ai/assistant`) and `models` underneath, used by every layer.
The chat adds one more outside system, reached the same way: **httpapi →
ai/assistant → ai/providers → the model's API**. Just as `store` is the only
package that writes SQL, `ai/providers` is the only package that talks to a model.

| Layer | Knows about | Never does |
|---|---|---|
| `httpapi` | requests, status codes, who is signed in | write SQL |
| `store` | SQL, transactions | decide business rules or build HTTP responses |
| `booking`, `leads`, `ai/assistant` | pure rules, no I/O | touch the database or the network |
| `ai/providers` | each provider's REST API | know about leads, slots or SQL |
| `models` | table shapes | contain behaviour |

Each domain follows the same file pattern, so a reader can guess where things are:

| Domain | Rules | SQL | HTTP |
|---|---|---|---|
| Booking | `booking/booking.go` | `store/booking.go` | `httpapi/booking_handlers.go` |
| Leads | `leads/*.go` | `store/leads.go`, `leads_query.go`, `leads_merge.go` | `httpapi/lead_handlers.go` |
| Sign-in | `auth/` | `store/users.go` | `httpapi/auth_handlers.go` |
| AI chat | `ai/assistant/*.go` (+ `ai/providers/` for the model) | `store/chat.go`, `chat_staff.go` | `httpapi/chat_handlers.go`, `chat_actions.go` |

### How one chat message flows

```
buyer ──POST /api/chat/messages──▶ httpapi/chat_handlers.go   finds the chat by its cookie, saves the message
                                        │
                                        ▼
                                   ai/assistant/agent.go       loop: ask the model, run the tools it asks for
                                     │            │
                   ai.ChatCompletion │            │ assistant.Actions
                                     ▼            ▼
                         ai/providers/*.go       httpapi/chat_actions.go ──▶ store (booking engine, leads)
                 (whichever AI_PROVIDER picks)
                                     │
                                     ▼
                              ai/assistant/guard.go       checks the reply (prices, discounts, guarantees)
                                        │
buyer ◀────────── reply, slot cards, visit ─────┘
```

The agent only knows two interfaces, `ai.ChatCompletion` and `Actions`, so its
tests swap in a scripted model and fake actions and run in milliseconds.

### The `ai/` folder: one interface, one struct per provider

Go has no classes, so the "base class" is an **interface**: `ai.ChatCompletion`
in `ai/completion.go`. Each provider is a struct that has all of its methods:

| Method | `Gemini` | `OpenAI` | `Offline` |
|---|---|---|---|
| `Complete(ctx, Request) (Turn, error)` | POST to Gemini's REST API | POST to `/chat/completions` | always fails, so fixed replies are used |
| `Provider() string` | `"gemini"` | `"openai"` | `"offline"` |
| `Model() string` | e.g. `gemini-3.5-flash-lite` | e.g. `gpt-5-nano` | `"none"` |

`providers.New(cfg)` reads `AI_PROVIDER` and `AI_MODEL` and returns the right
struct. Nothing else in the app names a provider, so switching is a config
change only. Adding a provider means one new file in `ai/providers/` and one
line in `New`.

### Tests

All tests live in `backend/tests/`, in folders that mirror `internal/`, so the
tests for `internal/httpapi` are in `tests/httpapi`:

```
tests/
├── ai/
│   ├── assistant/     agent loop, guardrails, and the eval set (testdata/evals.json)
│   └── providers/     Gemini and OpenAI JSON translation, the provider picker
├── auth/  booking/  leads/  reminders/      pure-rule unit tests
├── httpapi/           the API end to end, including the security tests
├── store/             database integration tests (booking races, dedupe, merge)
└── testdb/            helper: a throw-away database schema for integration tests
```

Because the tests are in a separate folder, they are separate Go packages and can
use only what the code exports, exactly like any other caller. The few helpers
that tests check directly were made public for that, for example `httpapi.ClientIP`,
`providers.EncodeGeminiRequest` and `leads.EditDistance`.

Rule packages have fast unit tests; `store` and `httpapi` also have integration
tests that run against a throw-away schema in a real Postgres. Without
`DATABASE_URL` those print a loud SKIPPING banner; with `LEADFLOW_REQUIRE_DB=1`
(use it in CI) they fail instead, so a green run always means the database
tests really ran. Run everything with `go test ./... -timeout 30m` from `backend/`
(the database tests take about 11 minutes against the Sydney Supabase region,
longer than Go's default 10-minute limit).

## API at a glance

Everything is under `/api`. "Head" means the sales head only; "own" means an
executive sees only their own data (enforced in SQL).

| Who | Routes |
|---|---|
| Anyone | `GET /projects`, `GET /projects/{id}` (with fact sheet), `GET /slots?project=` |
| Buyer (visit link) | `GET /visits/{code}`, `POST /visits/{code}/cancel`, `/reschedule`, `/opt-out` |
| Buyer (chat cookie) | `POST /chat/start`, `GET /chat`, `POST /chat/messages`, `/chat/book`, `/chat/handoff` |
| Staff sign-in | `POST /auth/login`, `/auth/logout`, `GET /auth/me` |
| Staff (own) | leads: `GET/POST /staff/leads`, `POST /staff/leads/import`, `GET /staff/leads/{id}`, `POST /staff/leads/{id}/status`, `/opt-out`; visits: `GET/POST /staff/visits`, `POST /staff/visits/{id}/cancel`, `/outcome`; `GET /staff/slots`, `GET /staff/reminders`, `GET /staff/projects/{id}/facts` |
| Head | `GET /staff/dashboard`; `POST/DELETE /staff/slots`; `PUT/DELETE /staff/projects/{id}/facts/{topic}`; `GET/PUT /staff/spend`; `GET/POST /staff/users`, `POST /staff/users/{id}/active`, `/password`; `POST /staff/leads/{id}/assign`, `/duplicate`; `GET /staff/chats/flagged`, `POST /staff/chats/{id}/reviewed` |
| Messaging provider | `POST /webhooks/inbound` (shared secret; "STOP" opts out) |

### Reminders

When a visit is booked, two reminder rows are written (1 day and 2 hours
before). A background worker (`reminders/worker.go`) checks every 30 seconds
and sends the due ones through the `Messenger` interface. In this build that
is a mock that logs; the sent text is kept on the row for the staff reminder
log. Each reminder is claimed with `FOR UPDATE SKIP LOCKED`, so several
servers can run the worker and none sends a reminder twice. Consent, opt-out,
cancellation and lateness are checked again at send time, and every message
carries the RERA number and "reply STOP".

## Where each security check lives

Security is not one folder: each layer checks what it is responsible for, so
no single mistake opens everything. This table is the map.

| Who are you? | Check | Lives in |
|---|---|---|
| Staff | Password (bcrypt, same timing for unknown, wrong and disabled accounts) | `httpapi/auth_handlers.go`, `auth/auth.go` |
| Staff | Session cookie: random 256-bit token, only its hash stored, 12 h expiry, checked against `users.active` on every request | `auth/middleware.go`, `store/users.go` |
| Staff | Role: head-only routes | `auth.RequireRole` in `httpapi/router.go` |
| Staff | Executives see only their own leads | the SQL in `store/leads_query.go` |
| Buyer | Chat cookie: random token, only its hash stored, expires on the server after 30 days | `store/chat.go` |
| Buyer | A typed phone number proves nothing: a chat sees only its own name and visit, cannot book for or overwrite a lead that existed before it | `store/chat.go`, `httpapi/chat_actions.go` |
| Buyer | Visit page: unguessable 24-character code | `booking.NewVisitCode` |
| The AI | Tools only act on the current chat; slot ids must come from `get_free_slots`; every reply is checked for prices, discounts and guarantees | `ai/assistant/tools.go`, `ai/assistant/guard.go` |

| Attack | Defence | Lives in |
|---|---|---|
| Password guessing | 10 attempts/min per IP, and 5 failures per 15 min per account from anywhere | `httpapi/auth_handlers.go` |
| Faking your IP to dodge limits | Forwarding headers are ignored unless `TRUSTED_PROXY_HEADER` names the platform's one header | `ClientIP` in `httpapi/helpers.go` |
| CSRF | `SameSite=Lax` cookies, plus an Origin check on every state-changing request | `SameOrigin` in `httpapi/helpers.go` |
| Cookies over plain HTTP | `Secure` is on unless `APP_ENV=development`; an unset or misspelt `APP_ENV` is production or an error, never "insecure" | `config/config.go` |
| Cached personal data | `Cache-Control: no-store` and a deny-all CSP on every `/api` response; HSTS when on HTTPS | `httpapi/helpers.go` |
| Outages disguised as "wrong password" | Database errors return 500 and are logged | `auth/middleware.go`, `httpapi/auth_handlers.go` |
| Script injection in the web app | Strict Content-Security-Policy on every page: only our own scripts, no inline scripts | `httpapi/static.go` |
| Messaging someone who said no (DPDP) | Consent and opt-out re-checked when each reminder is sent; opt-out from the visit page, a STOP reply, or staff | `reminders/decide.go`, `store/reminders.go` |
| Forged "STOP" webhook calls | Shared secret compared in constant time; endpoint off without one | `httpapi/optout_handlers.go` |
| A departed employee | Switching a user off ends their sessions at once and withdraws their free slots | `store/users.go` |
| Wiping or opening production by accident | `cmd/seed` refuses `-reset` and the built-in demo password when `APP_ENV=production` | `cmd/seed/main.go` |

## Credits

The 3D tour's photo-scanned textures (wood, fabrics, plaster, tiles) are from [Poly Haven](https://polyhaven.com) under CC0. The white marble, vitrified floor tiles, brushed and powder-coated metal, speckled stoneware, the fabric weave and quilting, and the picture on the TV are generated in code (seamless noise), so no other assets are needed. Desktop browsers load 1024 px versions; phones and slower devices load the 512 px `*_s.webp` copies.

The tour is lit as a warm early evening: a low golden sun, a dusk sky outside the windows, and a warm light at every lamp (desktops only; phones get the same palette without the extra lights).
