# Deployment and running guide

> Draft. The "How it is deployed" section is completed in the final phase.

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

## Run it locally

Requirements: Go 1.22 or newer, Node 20 or newer, and a Postgres database (the project uses Neon).

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

Run the backend tests with `go test ./...` in `backend`.

## Environment variables (names only)

| Name | Used for |
|---|---|
| `DATABASE_URL` | Direct database connection, used by migrations and seeding |
| `DATABASE_URL_POOLED` | Pooled database connection, used by the running app |
| `GEMINI_API_KEY` | Google Gemini API key for the chat assistant |
| `PORT` | HTTP port (default 8080) |
| `APP_ENV` | `development` or `production` (production turns on secure cookies) |
| `ALLOWED_ORIGINS` | Extra browser origins allowed to send POST requests (default `http://localhost:5173` in development, none in production) |
| `SEED_DEMO_PASSWORD` | Optional: overrides the demo password when seeding |
| `VITE_SHOW_DEMO_LOGINS` | Optional frontend build flag: `true` shows the demo-account buttons on the login page |

Never commit `backend/.env`. It is listed in `.gitignore`.

## How it is deployed

To be completed.
