# AGENTS.md

Go 1.26 Telegram bot ("Распиши-ка") that shows МПК ТИУ college schedules. UI strings, comments, and .env values are in Russian — keep user-facing bot text in Russian.

## Architecture

- `cmd/bot` — the Telegram bot (the main binary). Persists to SQLite, renders schedule screenshots via its own chromedp browser (`internal/browser`), talks to the scraper API over HTTP (`internal/apiclient`).
- `cmd/api` — HTTP scraper API (`internal/api/*`). Scrapes `coworking.tyuiu.ru` over plain HTTP (`internal/api/scraper`), caches in Redis, exposes `/api/v1/*` + Swagger UI (gin).
- `cmd/fakeapi` — same API server but with hardcoded fake data (`fake_scraper.go`).
- Bot connects to Telegram via SOCKS5 proxies fetched at runtime from a proxy source (filtered to non-RU socks5, cached 1h in memory). No proxies → `ErrNoAvailableProxy`.

## External modules

- The proxy stack and the bot lifecycle manager live in `github.com/azzimoda/go-tg-proxy` (packages `proxy`, `botservice`, `proxyutil`), pinned at `v0.1.0` here. Its source provider is injected (`proxy.NewProxiflySource`), the default URL is proxifly's jsdelivr mirror, overridable with `PROXY_SOURCE_URL`.
- After pushing changes to the module, bump the version here with `go get github.com/azzimoda/go-tg-proxy@<version>` and `go mod tidy`.

## Build & run

- Bot needs CGO (sqlite3) and a Chromium binary on PATH (chromedp): `go build ./cmd/bot`
- API needs no extra browsers: it scrapes `coworking.tyuiu.ru` over plain HTTP.

- Local dev with demo data: `docker compose up --build` runs redis + fakeapi + bot.
- Run the real scraper instead: `go run ./cmd/api` (needs Redis).
- TZ everywhere is `Asia/Yekaterinburg`.

## Config

viper + godotenv (`.env` at repo root), defaults in `pkg/config/config.go`, keys are env-var-style (e.g. `BOT_TOKEN`, `SCRAPER_HOST`). `config.Init()` must run before viper reads. Notable keys:

- `BOT_TOKEN` (required), `ADMIN_BOT_TOKEN` + `ADMIN_ID` (enables admin bot), `SCRAPER_HOST`/`SCRAPER_PORT`, `LOG_LEVEL` (`trace` enables bot debug output), `BROWSER_SCALE`, `HANDLE_VACATION`.

## Database

SQLite at `storage/database/data.db`, accessed via GORM (`database.Open` returns `*gorm.DB`). Migrations are managed by goose (`github.com/pressly/goose/v3`): `migrations/NNNNN_name.sql` files with a `-- +goose Up` annotation are applied automatically on open and tracked in the `goose_db_version` table — never edit an already-applied migration; add a new numbered file instead.

## Tests

- `go test ./...` passes with no external services (Redis tests use miniredis, HTTP tests use httptest). Run from repo root: some tests (`internal/model`) chdir to project root via `testutil.MoveToProjectRoot()`.
- `go build ./...` should stay clean. There is no linter/CI/Makefile.

## Codegen

Swagger docs in `docs/` are generated, not hand-written. After changing API annotations in `cmd/api/main.go` or `cmd/fakeapi/main.go`, run `go generate ./...` (requires `swag`, e.g. `go install github.com/swaggo/swag/cmd/swag@v1.16.6`).
