# AGENTS.md

Go 1.26 Telegram bot ("Распиши-ка") that shows МПК ТИУ college schedules. UI strings, comments, and .env values are in Russian — keep user-facing bot text in Russian.

## Architecture

- `cmd/bot` — the Telegram bot (the main binary). Persists to SQLite, renders schedule screenshots via its own chromedp browser (`internal/browser`), talks to the scraper API over HTTP (`internal/apiclient`).
- `cmd/api` — HTTP scraper API (`internal/api/*`). Scrapes `coworking.tyuiu.ru` over plain HTTP (`internal/api/scraper`), caches in Redis, exposes `/api/v1/*` + Swagger UI (gin).
- `cmd/fakeapi` — same API server but with hardcoded fake data (`fake_scraper.go`).
- `cmd/justray-rotate` — host-side daemon that watches the local justray in-bound (`JUSTRAY_PROXY_ADDR`) and rotates justray's active node (round-robin over non-RU alive nodes from `justray subscription list --json`) after `JUSTRAY_FAILURE_THRESHOLD` failed probes to `JUSTRAY_PROBE_URL`. Backs off (`JUSTRAY_COOLDOWN`, `JUSTRAY_LONG_BACKOFF` after `JUSTRAY_MAX_ROTATIONS`). Install as a systemd user unit with `make install-justray-rotate` (unit template `configs/justray-rotate.service`, built by `make build-justray-rotate`); `make uninstall-justray-rotate` removes it. Runtime smoke: `RT_SMOKE=1`.
- `cmd/migrate` — one-shot binary that applies the goose migrations and exits. In Docker it is the `migrate` service (`restart: "no"`); the bot, adminbot and vkbot set `DB_AUTO_MIGRATE=false` and wait for it with `depends_on: migrate: condition: service_completed_successfully`, so the migration runs once instead of racing four processes on the same SQLite file.
- Bot connects to Telegram via SOCKS5 proxies fetched at runtime from a proxy source (filtered to non-RU socks5, cached 1h in memory). No proxies → `ErrNoAvailableProxy`.

## External modules

- The proxy stack and the bot lifecycle manager live in `github.com/azzimoda/go-tg-proxy` (packages `proxy`, `botservice`, `proxyutil`), pinned at `v0.1.2` here. Its source provider is injected (`proxy.NewProxiflySource`), the default URL is proxifly's jsdelivr mirror, overridable with `PROXY_SOURCE_URL`. A primary local proxy is layered on top: `internal/service/newJustrayFirstSource` prepends `JUSTRAY_PROXY_ADDR` (default `127.0.0.1:10808`, e.g. a justray mixed in-bound in proxy mode) ahead of the free-proxy list. justray is checked first and, while it passes, the free proxies are never used: priority comes from a deterministic checker, not from latency ranking (an earlier version claimed justray won the ranking, which is not how `go-tg-proxy` picks — it ranks the free list by measured latency, so justray would have been picked only by luck). When justray is down the free list takes over, and the list stops being cached while it is degraded. In Docker, compose passes `host.docker.internal:10808` + `host-gateway`; set `JUSTRAY_PROXY_ADDR=` empty to keep only the free-proxy source.
- After pushing changes to the module, bump the version here with `go get github.com/azzimoda/go-tg-proxy@<version>` and `go mod tidy`.

## Build & run

- Bot needs CGO (sqlite3) and a Chromium binary on PATH (chromedp): `go build ./cmd/bot`
- API needs no extra browsers: it scrapes `coworking.tyuiu.ru` over plain HTTP.

- Local dev with demo data: `docker compose up --build` runs redis + fakeapi + bot.
- Run the real scraper instead: `go run ./cmd/api` (needs Redis).
- TZ everywhere is `Asia/Yekaterinburg`.

## Config

viper + godotenv (`.env` at repo root), defaults in `pkg/config/config.go`, keys are env-var-style (e.g. `BOT_TOKEN`, `SCRAPER_HOST`). `config.Init()` must run before viper reads. Notable keys:

- `BOT_TOKEN` (required), `ADMIN_BOT_TOKEN` + `ADMIN_ID` (enables admin bot), `SCRAPER_HOST`/`SCRAPER_PORT`, `PROXY_SOURCE_URL`, `JUSTRAY_PROXY_ADDR`, `LOG_LEVEL` (`trace` enables bot debug output), `BROWSER_SCALE`, `HANDLE_VACATION`, `VK_GROUP_TOKEN`/`VK_GROUP_ID`/`VK_API_VERSION`, `DB_AUTO_MIGRATE`, `IMAGE_TAG` (image tag the deploy uses; defaults to `latest`). The rotator reads the same env: `JUSTRAY_BIN`, `JUSTRAY_PROBE_URL`, `JUSTRAY_CHECK_INTERVAL`, `JUSTRAY_FAILURE_THRESHOLD`, `JUSTRAY_COOLDOWN`, `JUSTRAY_MAX_ROTATIONS`, `JUSTRAY_LONG_BACKOFF`, `JUSTRAY_EXCLUDE` (extra comma-separated substrings to exclude; 🇷🇺/«Россия»/«моб. операторов» excluded by default).

## Database

SQLite at `storage/database/data.db`, accessed via GORM (`database.Open` returns `*gorm.DB`). Migrations are managed by goose (`github.com/pressly/goose/v3`): `migrations/NNNNN_name.sql` files with a `-- +goose Up` annotation, tracked in the `goose_db_version` table — never edit an already-applied migration; add a new numbered file instead.

`DB_AUTO_MIGRATE` (default `true`) decides whether `database.Open` runs `goose.Up` itself. Leave it on for local runs, and set it to `false` everywhere several processes share one file: in Docker the `migrate` service applies them and everything else waits for it. The postgres set lives in `migrations/postgres/`.

## Tests

- `go test ./...` passes with no external services (Redis tests use miniredis, HTTP tests use httptest). Run from repo root: some tests (`internal/model`) chdir to project root via `testutil.MoveToProjectRoot()`.
- `go build ./...` should stay clean. There is a repo-root `Makefile`: `make check` runs `gofmt -l` + `go vet` + `go test` + `go build`, `make docs` regenerates Swagger, `make up-fake`/`make up-local` run the demo/real stack in Docker. `make test-race` is `go test -race ./...` and `make test-pg` runs the two PostgreSQL-gated packages against `TEST_PG_DSN` (skipped when unset); the postgres tests clean up their scratch rows, so both are re-runnable. `make check` is the same gate CI runs. There is no linter locally.

## Codegen

Swagger docs in `docs/` are generated, not hand-written. After changing API annotations in `cmd/api/main.go` or `cmd/fakeapi/main.go`, run `go generate ./...` (requires `swag`, e.g. `go install github.com/swaggo/swag/cmd/swag@v1.16.6`).
