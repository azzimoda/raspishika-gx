GO       ?= go
COMPOSE  ?= docker compose
VERSION  ?=

# Stack variants, selected by the up-*/down-* pair. The down targets used to run
# bare `docker compose`, so tearing down after `make up-fake` or `make up-local`
# did not see the same project and left containers running.
STACK       ?=
STACK_FLAG   = $(if $(STACK),-f compose.$(STACK).yaml,)

# Compose stacks: up-fake/up-local, torn down with down-fake-ro/down-local-ro.
.PHONY: all help deps fmt fmt-check vet build build-bot build-adminbot build-api build-fakeapi build-fakebot build-vkbot build-fakevkbot build-justray-rotate install-justray-rotate uninstall-justray-rotate install-justray-inbound-proxy uninstall-justray-inbound-proxy test test-race test-pg docs check run-api run-fakeapi run-fakebot run-adminbot run-vkbot run-fakevkbot bump-proxy up up-fake up-local down down-fake-ro down-local-ro rollback logs clean

BIN_DIR   ?= $(HOME)/.local/bin
UNIT_DIR  ?= $(HOME)/.config/systemd/user
UNIT_NAME := justray-rotate.service

# The docker bridge to justray forwarder, and the justray in-bound port it
# serves. Override PORT only if justray's own connection.port was changed.
INBOUND_UNIT_SOCKET  := justray-inbound-proxy.socket
INBOUND_UNIT_SERVICE := justray-inbound-proxy.service
INBOUND_PORT         ?= 10808

all: check

help:
	@echo "Makefile targets:"
	@echo "  deps         download Go modules (go mod download)"
	@echo "  fmt          format Go code (gofmt -w internal/ cmd/ pkg/)"
	@echo "  fmt-check    list files that need formatting (gofmt -l)"
	@echo "  vet          go vet ./..."
	@echo "  build        go build ./..."
	@echo "  build-bot    build only the bot (./cmd/bot, needs CGO and Chromium)"
	@echo "  build-adminbot  build the admin bot (./cmd/adminbot)"
	@echo "  build-api    build the scraper API (./cmd/api)"
	@echo "  build-fakeapi  build the demo API (./cmd/fakeapi)"
	@echo "  build-fakebot  build the API-less demo bot (./cmd/fakebot)"
	@echo "  build-vkbot    build the VK community bot (./cmd/vkbot)"
	@echo "  build-fakevkbot  build the API-less demo VK bot (./cmd/fakevkbot)"
	@echo "  build-justray-rotate  build the justray node rotator (./cmd/justray-rotate)"
	@echo "  install-justray-rotate  build + install rotator as a systemd user unit"
	@echo "  uninstall-justray-rotate  stop and remove the rotator systemd unit"
	@echo "  install-justray-inbound-proxy  bridge the docker network to justray's loopback in-bound"
	@echo "  uninstall-justray-inbound-proxy  remove the in-bound bridge units"
	@echo "  test         go test ./..."
	@echo "  test-pg       Postgres-backed tests (TEST_PG_DSN=... to point elsewhere)"
	@echo "  test-race    go test -race (bot and fakescraper)"
	@echo "  check        fmt-check + vet + test + build"
	@echo "  docs         regenerate Swagger docs (go generate ./...)"
	@echo "  run-api      go run ./cmd/api (needs Redis)"
	@echo "  run-fakeapi  go run ./cmd/fakeapi"
	@echo "  run-fakebot  go run ./cmd/fakebot"
	@echo "  run-adminbot go run ./cmd/adminbot (needs the stack: DB, API, ADMIN_ID)"
	@echo "  run-vkbot    go run ./cmd/vkbot (needs VK credentials, DB, API)"
	@echo "  run-fakevkbot  go run ./cmd/fakevkbot (needs VK credentials, DB)"
	@echo "  bump-proxy VERSION=v0.x.y  update github.com/azzimoda/go-tg-proxy"
	@echo "  up           docker compose up --build -d"
	@echo "  up-fake      docker compose -f compose.fakeapi.yaml up --build -d"
	@echo "  up-local     docker compose -f compose.local.yaml up --build -d"
	@echo "  down         docker compose down (STACK=<fakeapi|local> to match a variant stack)"
	@echo "  logs         docker compose logs -f"
	@echo "  rollback TAG=<sha>  redeploy a previously deployed image by SHA"
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

build-adminbot:
	$(GO) build ./cmd/adminbot

build-api:
	$(GO) build ./cmd/api

build-fakeapi:
	$(GO) build ./cmd/fakeapi

build-fakebot:
	$(GO) build ./cmd/fakebot

build-vkbot:
	$(GO) build ./cmd/vkbot

build-fakevkbot:
	$(GO) build ./cmd/fakevkbot

# Build straight into BIN_DIR. Staging in /tmp first would leave a window in
# which another local user could replace the binary between build and install.
build-justray-rotate:
	@mkdir -p $(BIN_DIR)
	$(GO) build -o $(BIN_DIR)/justray-rotate ./cmd/justray-rotate

install-justray-rotate: build-justray-rotate
	@mkdir -p $(UNIT_DIR)
	sed -e 's|__REPO_DIR__|$(CURDIR)|' -e 's|__BIN_DIR__|$(BIN_DIR)|' configs/justray-rotate.service > $(UNIT_DIR)/$(UNIT_NAME)
	systemctl --user daemon-reload
	systemctl --user enable --now justray-rotate.service
	@# Without linger the user manager is killed on logout and the rotator stops.
	@loginctl enable-linger $$(id -un) 2>/dev/null || echo "warning: could not enable linger; the rotator will stop on logout"
	@echo "Installed: $(BIN_DIR)/justray-rotate and $(UNIT_DIR)/$(UNIT_NAME)"

uninstall-justray-rotate:
	systemctl --user disable --now justray-rotate.service || true
	rm -f $(UNIT_DIR)/$(UNIT_NAME) $(BIN_DIR)/justray-rotate
	systemctl --user daemon-reload
	@echo "Removed justray-rotate unit and binary"

# justray with `allow_lan` off listens on 127.0.0.1 only, so the containers
# cannot reach it. This installs a systemd socket-proxyd pair on the docker
# bridge gateway, which is what host.docker.internal resolves to in the
# containers. Nothing then listens on the host's public interface, so no
# firewall rule is needed for the port.
install-justray-inbound-proxy:
	@set -e; \
	bridge=$$(docker network inspect bridge --format '{{(index .IPAM.Config 0).Gateway}}' 2>/dev/null || true); \
	if [ -z "$$bridge" ] || [ "$$bridge" = "0.0.0.0" ]; then \
		echo "error: cannot use the docker bridge gateway (got '$$bridge')."; \
		echo "       Start docker, and never let this fall back to 0.0.0.0: that publishes an open SOCKS5."; \
		exit 1; \
	fi; \
	proxyd=$$(command -v systemd-socket-proxyd 2>/dev/null || true); \
	if [ -z "$$proxyd" ]; then proxyd=/lib/systemd/systemd-socket-proxyd; fi; \
	if [ ! -x "$$proxyd" ]; then \
		echo "error: systemd-socket-proxyd not found at $$proxyd."; \
		echo "       Install the systemd package, or bridge the port with socat instead."; \
		exit 1; \
	fi; \
	mkdir -p $(UNIT_DIR); \
	sed -e "s|__BRIDGE_IP__|$$bridge|g" -e "s|__PORT__|$(INBOUND_PORT)|g" configs/justray-inbound-proxy.socket > $(UNIT_DIR)/$(INBOUND_UNIT_SOCKET); \
	sed -e "s|__PROXYD__|$$proxyd|g" -e "s|__PORT__|$(INBOUND_PORT)|g" configs/justray-inbound-proxy.service > $(UNIT_DIR)/$(INBOUND_UNIT_SERVICE); \
	systemctl --user daemon-reload; \
	systemctl --user enable --now $(INBOUND_UNIT_SOCKET); \
	loginctl enable-linger $$(id -un) 2>/dev/null || echo "warning: could not enable linger; the bridge stops on logout"; \
	echo "Forwarding $$bridge:$(INBOUND_PORT) -> 127.0.0.1:$(INBOUND_PORT)"; \
	echo "Point the containers at host.docker.internal:$(INBOUND_PORT) (JUSTRAY_PROXY_ADDR)"

uninstall-justray-inbound-proxy:
	systemctl --user disable --now $(INBOUND_UNIT_SOCKET) || true
	systemctl --user stop $(INBOUND_UNIT_SERVICE) 2>/dev/null || true
	rm -f $(UNIT_DIR)/$(INBOUND_UNIT_SOCKET) $(UNIT_DIR)/$(INBOUND_UNIT_SERVICE)
	systemctl --user daemon-reload
	@echo "Removed the justray in-bound bridge units"

test:
	$(GO) test ./...

# The whole tree, not just the packages that used to have concurrent tests: the
# VK client, the rotator and the service layer all start goroutines of their own.
test-race:
	$(GO) test -race ./...

# Postgres-backed tests are skipped unless TEST_POSTGRES_DSN is set, so the
# dialect-specific SQL (IS TRUE on booleans, derived-table aliases) is never
# exercised by a plain `make test`. Point this at a scratch database: the tests
# write rows and delete them again.
TEST_PG_DSN ?= host=127.0.0.1 port=5432 user=postgres password=raspishika dbname=raspishika sslmode=disable
test-pg:
	TEST_POSTGRES_DSN="$(TEST_PG_DSN)" $(GO) test -count=1 -v -run Postgres ./pkg/database/ ./internal/repository/

check: fmt-check vet test build

docs:
	$(GO) generate ./...

run-api:
	$(GO) run ./cmd/api

run-fakeapi:
	$(GO) run ./cmd/fakeapi

run-fakebot:
	$(GO) run ./cmd/fakebot

run-adminbot:
	$(GO) run ./cmd/adminbot

run-vkbot:
	$(GO) run ./cmd/vkbot

run-fakevkbot:
	$(GO) run ./cmd/fakevkbot

bump-proxy:
	@test -n "$(VERSION)" || { echo "Usage: make bump-proxy VERSION=v0.x.y"; exit 1; }
	$(GO) get github.com/azzimoda/go-tg-proxy@$(VERSION)
	$(GO) mod tidy

up:
	$(COMPOSE) $(STACK_FLAG) up --build -d

up-fake:
	$(MAKE) up STACK=fakeapi

up-local:
	$(MAKE) up STACK=local

down:
	$(COMPOSE) $(STACK_FLAG) down

down-ro:
	$(COMPOSE) $(STACK_FLAG) down --remove-orphans

down-fake-ro:
	$(MAKE) down-ro STACK=fakeapi

down-local-ro:
	$(MAKE) down-ro STACK=local

# Откат на ранее задеплоенный SHA (образы тегируются по SHA в CI).
# Выполняется на VPS, где лежит рабочий клон и .env.
rollback:
	@test -n "$(TAG)" || { echo "Usage: make rollback TAG=<sha>"; exit 1; }
	sed -i '/^IMAGE_TAG=/d' .env
	echo "IMAGE_TAG=$(TAG)" >> .env
	$(COMPOSE) up -d
	$(COMPOSE) ps

logs:
	$(COMPOSE) logs -f

clean:
	$(GO) clean
	$(COMPOSE) down
