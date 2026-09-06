GO       ?= go
COMPOSE  ?= docker compose
VERSION  ?=

.PHONY: all help deps fmt fmt-check vet build build-bot build-api build-fakeapi build-fakebot test test-race docs check run-api run-fakeapi run-fakebot bump-proxy up up-fake up-local down logs clean

all: check

help:
	@echo "Makefile targets:"
	@echo "  deps         download Go modules (go mod download)"
	@echo "  fmt          format Go code (gofmt -w internal/ cmd/ pkg/)"
	@echo "  fmt-check    list files that need formatting (gofmt -l)"
	@echo "  vet          go vet ./..."
	@echo "  build        go build ./..."
	@echo "  build-bot    build only the bot (./cmd/bot, needs CGO and Chromium)"
	@echo "  build-api    build the scraper API (./cmd/api)"
	@echo "  build-fakeapi  build the demo API (./cmd/fakeapi)"
	@echo "  build-fakebot  build the API-less demo bot (./cmd/fakebot)"
	@echo "  test         go test ./..."
	@echo "  test-race    go test -race (bot and fakescraper)"
	@echo "  check        fmt-check + vet + test + build"
	@echo "  docs         regenerate Swagger docs (go generate ./...)"
	@echo "  run-api      go run ./cmd/api (needs Redis)"
	@echo "  run-fakeapi  go run ./cmd/fakeapi"
	@echo "  run-fakebot  go run ./cmd/fakebot"
	@echo "  bump-proxy VERSION=v0.x.y  update github.com/azzimoda/go-tg-proxy"
	@echo "  up           docker compose up --build -d"
	@echo "  up-fake      docker compose -f compose.fakeapi.yaml up --build -d"
	@echo "  up-local     docker compose -f compose.local.yaml up --build -d"
	@echo "  down         docker compose down"
	@echo "  logs         docker compose logs -f"
	@echo "  clean        go clean + docker compose down"

deps:
	$(GO) mod download

fmt:
	gofmt -w internal/ cmd/ pkg/

fmt-check:
	@test -z "$$(gofmt -l internal/ cmd/ pkg/)" || { echo "gofmt: файлы требуют форматирования:"; gofmt -l internal/ cmd/ pkg/; exit 1; }

vet:
	$(GO) vet ./...

build:
	$(GO) build ./...

build-bot:
	$(GO) build ./cmd/bot

build-api:
	$(GO) build ./cmd/api

build-fakeapi:
	$(GO) build ./cmd/fakeapi

build-fakebot:
	$(GO) build ./cmd/fakebot

test:
	$(GO) test ./...

test-race:
	$(GO) test -race ./internal/bot/... ./internal/fakescraper/...

check: fmt-check vet test build

docs:
	$(GO) generate ./...

run-api:
	$(GO) run ./cmd/api

run-fakeapi:
	$(GO) run ./cmd/fakeapi

run-fakebot:
	$(GO) run ./cmd/fakebot

bump-proxy:
	@test -n "$(VERSION)" || { echo "Usage: make bump-proxy VERSION=v0.x.y"; exit 1; }
	$(GO) get github.com/azzimoda/go-tg-proxy@$(VERSION)
	$(GO) mod tidy

up:
	$(COMPOSE) up --build -d

up-fake:
	$(COMPOSE) -f compose.fakeapi.yaml up --build -d

up-local:
	$(COMPOSE) -f compose.local.yaml up --build -d

down:
	$(COMPOSE) down

logs:
	$(COMPOSE) logs -f

clean:
	$(GO) clean
	$(COMPOSE) down