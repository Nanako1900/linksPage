# LinksPage developer tasks. Run `make help` for a list.
SHELL := /bin/bash
.SHELLFLAGS := -eu -o pipefail -c

PNPM      ?= corepack pnpm
WEB       := web
WEBUI_DIST := internal/webui/dist
VERSION   ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT    ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
LDFLAGS   := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT)
IMAGE     ?= ghcr.io/nanako1900/linkspage
TAG       ?= dev
GOLANGCI  ?= golangci-lint
SQLC      ?= docker run --rm -v "$(CURDIR)":/src -w /src sqlc/sqlc:1.31.1

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*?## "}{printf "  %-14s %s\n",$$1,$$2}'

.PHONY: web-install
web-install: ## Install frontend dependencies (frozen lockfile)
	cd $(WEB) && $(PNPM) install --frozen-lockfile

.PHONY: web-build
web-build: ## Build the frontend into web/dist
	cd $(WEB) && $(PNPM) build

.PHONY: web-dist
web-dist: web-build ## Copy web/dist into internal/webui/dist for go:embed
	find $(WEBUI_DIST) -mindepth 1 ! -name .gitkeep -exec rm -rf {} +
	cp -R $(WEB)/dist/. $(WEBUI_DIST)/

.PHONY: web-dist-clean
web-dist-clean: ## Empty internal/webui/dist (keeps .gitkeep)
	find $(WEBUI_DIST) -mindepth 1 ! -name .gitkeep -exec rm -rf {} +

.PHONY: build
build: web-dist ## Build bin/linkspage with the frontend embedded
	CGO_ENABLED=0 go build -trimpath -tags nodynamic -ldflags "$(LDFLAGS)" -o bin/linkspage ./cmd/linkspage

.PHONY: gen
gen: ## Regenerate sqlc code, web/openapi.json and the orval client
	$(SQLC) generate
	go run ./cmd/linkspage openapi > $(WEB)/openapi.json
	cd $(WEB) && $(PNPM) gen:api

.PHONY: gen-check
gen-check: gen ## Fail if generated files differ from the committed ones
	git diff --exit-code -- $(WEB)/openapi.json $(WEB)/src/shared/api/gen internal/store/dbq
	test -z "$$(git status --porcelain -- $(WEB)/src/shared/api/gen internal/store/dbq)"

.PHONY: lint
lint: ## Lint Go and web code
	$(GOLANGCI) run ./...
	@! grep -rn 'style=' internal/webui/templates || (echo "style= attributes are not allowed in Go templates" >&2; exit 1)
	cd $(WEB) && $(PNPM) lint && $(PNPM) typecheck

.PHONY: test
test: ## Run Go and web tests
	go test -race -cover ./...
	cd $(WEB) && $(PNPM) test

.PHONY: docker
docker: ## Build the container image ($(IMAGE):$(TAG))
	docker build --build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) -t $(IMAGE):$(TAG) .

.PHONY: dev-db
dev-db: ## Start a local PostgreSQL for development
	docker compose -f deploy/compose.dev.yaml up -d db

.PHONY: dev
dev: dev-db ## Run db, the Go server (air if installed) and the Vite dev server
	@trap 'kill 0' EXIT; \
	( if command -v air >/dev/null; then air; else \
	  LP_BASE_URL=http://localhost:5173 LP_DB__HOST=127.0.0.1 LP_DB__PORT=55432 LP_DB__USER=linkspage \
	  LP_DB__NAME=linkspage LP_DB__PASSWORD=linkspage-dev LP_DATA_DIR=./tmp/data LP_LOG__FORMAT=text \
	  go run ./cmd/linkspage serve; fi ) & \
	( cd $(WEB) && $(PNPM) dev ) & \
	wait
