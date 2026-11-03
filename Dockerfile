# One image runs the whole app: the Go server serves the API and the built
# React files from the same origin (so no CORS, and cookies just work).
#
#   docker build -t leadflow .
#   docker run -p 8080:8080 --env-file backend/.env -e APP_ENV=production leadflow
#
# On start it applies any pending database migrations, then starts the server.

# ---- 1. Build the React app ---------------------------------------------------
FROM node:22-alpine AS web
WORKDIR /web
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# ---- 2. Build the Go binaries -------------------------------------------------
FROM golang:1.27-alpine AS api
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
# CGO off gives static binaries that run on a bare Alpine image.
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server \
 && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/migrate ./cmd/migrate \
 && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/seed ./cmd/seed

# ---- 3. The small image that runs -------------------------------------------
FROM alpine:3.22
# ca-certificates: HTTPS to the database and the AI provider. tzdata: time zones.
RUN apk add --no-cache ca-certificates tzdata \
 && adduser -D -H -u 10001 app
WORKDIR /app
COPY --from=api /out/ ./
COPY --from=web /web/dist ./web
USER app
ENV PORT=8080 STATIC_DIR=/app/web APP_ENV=production
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s CMD wget -qO- http://127.0.0.1:8080/healthz >/dev/null || exit 1
CMD ["sh", "-c", "./migrate && exec ./server"]
