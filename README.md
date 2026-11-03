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
│   └── internal/                application code, one package per concern:
│       ├── config/              environment variables
│       ├── models/              one struct per database table, plus allowed values
│       ├── auth/                passwords, sessions, who-may-do-what middleware
│       ├── booking/             pure rules: notice period, reminder times, slot checks
│       ├── leads/               pure rules: phone normalising, name matching, merging
│       ├── store/               ALL SQL lives here (booking*, leads*, users)
│       ├── httpapi/             routes, handlers, request checks
│       ├── seed/                the sample-data seeder
│       └── testdb/              throw-away database schema for integration tests
├── frontend/                    React + TypeScript (Vite)
│   └── src/
│       ├── api/                 the one place that talks to the backend
│       ├── auth/                sign-in state and route guards
│       ├── pages/               one file per screen (+ its CSS)
│       └── index.css            design tokens: colours, type, spacing
├── deployment.md                run, deploy, env var names, demo logins
└── HANDOFF.md                   session notes for whoever continues the work
```

## How the backend is organised

Code flows one way: **httpapi → store → database**, with the rule packages
(`booking`, `leads`) and `models` underneath, used by every layer.

| Layer | Knows about | Never does |
|---|---|---|
| `httpapi` | requests, status codes, who is signed in | write SQL |
| `store` | SQL, transactions | decide business rules or build HTTP responses |
| `booking`, `leads` | pure rules, no I/O | touch the database or the network |
| `models` | table shapes | contain behaviour |

Each domain follows the same file pattern, so a reader can guess where things are:

| Domain | Rules | SQL | HTTP |
|---|---|---|---|
| Booking | `booking/booking.go` | `store/booking.go` | `httpapi/booking_handlers.go` |
| Leads | `leads/*.go` | `store/leads.go`, `leads_query.go`, `leads_merge.go` | `httpapi/lead_handlers.go` |
| Sign-in | `auth/` | `store/users.go` | `httpapi/auth_handlers.go` |

Tests sit beside the code they test (`x.go` and `x_test.go`). Rule packages have
fast unit tests; `store` and `httpapi` also have integration tests that run
against a throw-away schema in a real Postgres and skip themselves when no
`DATABASE_URL` is set.
