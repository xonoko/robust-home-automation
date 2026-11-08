SHELL := /bin/bash
.DEFAULT_GOAL := help

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
IMAGE   ?= ghcr.io/xonoko/robust-home-automation/hub
# Build tools live in their own module (tools/go.mod) so their dependencies
# never constrain the hub's.
TOOL := go tool -modfile=tools/go.mod
LDFLAGS := -s -w \
	-X github.com/xonoko/robust-home-automation/internal/version.Version=$(VERSION) \
	-X github.com/xonoko/robust-home-automation/internal/version.Commit=$(COMMIT)

.PHONY: help
help: ## Show this help
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

.PHONY: generate
generate: ## Regenerate ent, protobuf and the web API client
	go generate ./...
	$(TOOL) buf generate
	go run ./cmd/hub openapi > web/openapi.json
	cd web && npm run gen:api

.PHONY: migration
migration: ## Create a migration from ent schema changes: make migration NAME=add_x
	@test -n "$(NAME)" || (echo "usage: make migration NAME=<name>" && exit 1)
	atlas migrate diff $(NAME) --env local

.PHONY: build
build: ## Build the hub binary into bin/
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/hub ./cmd/hub

.PHONY: web
web: ## Build the web UI (embedded into the binary on the next build)
	cd web && npm ci && npm run build

.PHONY: test
test: ## Run Go tests
	go test -race ./...

.PHONY: lint
lint: ## Lint Go and protobuf
	$(TOOL) golangci-lint run ./...
	$(TOOL) buf lint

.PHONY: check-generated
check-generated: generate ## Fail if generated code is out of date
	@git diff --exit-code -- internal/store/ent internal/gen web/openapi.json web/src/api || \
		(echo "generated code is out of date: run make generate and commit" && exit 1)

.PHONY: image
image: ## Build the amd64 container image
	docker build --platform linux/amd64 \
		--build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) \
		-t $(IMAGE):$(VERSION) .

.PHONY: run
run: ## Run the hub locally with text logs (needs a NATS server)
	HUB_TELEMETRY__LOG_FORMAT=text go run ./cmd/hub serve

.PHONY: dev-up
dev-up: ## Deploy everything to the Docker Desktop Kubernetes cluster
	./deploy/dev/up.sh

.PHONY: dev-down
dev-down: ## Remove the deployment from the Docker Desktop cluster
	./deploy/dev/down.sh
