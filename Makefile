SHELL := /bin/bash

GO_IMAGE := golang:1.26.5-bookworm
NODE_IMAGE := node:24.19.0-bookworm-slim
PLAYWRIGHT_IMAGE := mcr.microsoft.com/playwright:v1.62.1-noble
GITLEAKS_IMAGE := zricethezav/gitleaks:v8.30.1
SQLC_IMAGE := sqlc/sqlc:1.31.0
CONTAINER_USER := $(shell id -u):$(shell id -g)
ROOT := /workspace
VERSION ?= dev
REVISION ?= $(shell git rev-parse --short HEAD 2>/dev/null || printf unknown)
VERSION_PACKAGE := github.com/ipchronicle/ipchronicle/internal/version.Value
GO_RUN := docker run --rm --user $(CONTAINER_USER) -e HOME=/tmp -e GOCACHE=/tmp/go-build -e GOMODCACHE=/tmp/go-mod -v $(CURDIR):$(ROOT) -w $(ROOT) $(GO_IMAGE)
NODE_RUN := docker run --rm --user $(CONTAINER_USER) -e HOME=/tmp -v $(CURDIR):$(ROOT) -w $(ROOT)/web $(NODE_IMAGE)
SQLC_RUN := docker run --rm --user $(CONTAINER_USER) -v $(CURDIR):/src -w /src $(SQLC_IMAGE)

.PHONY: all browser-test build check compose-smoke format generate go-check secret-scan web-assets web-check

all: check

generate:
	$(GO_RUN) sh -ceu 'go mod download; go tool oapi-codegen -config openapi/oapi-codegen-server.yaml openapi/openapi.yaml; go tool oapi-codegen -config openapi/oapi-codegen-agent.yaml openapi/openapi.yaml'
	$(SQLC_RUN) generate
	$(NODE_RUN) sh -ceu 'npm ci; npm run generate:api'

format:
	$(GO_RUN) sh -ceu 'gofmt -w $$(find . -type f -name "*.go" -not -path "./web/node_modules/*")'
	$(NODE_RUN) sh -ceu 'npm ci; npm run format'

web-check:
	$(NODE_RUN) sh -ceu 'npm ci; npm run check'

web-assets: web-check
	./scripts/sync-web-assets.sh

go-check:
	$(GO_RUN) sh -ceu 'go mod tidy; unformatted=$$(gofmt -l $$(find . -type f -name "*.go" -not -path "./web/node_modules/*")); test -z "$$unformatted" || { printf "Unformatted Go files:\n%s\n" "$$unformatted"; exit 1; }; go vet ./...; go test ./...; go test -race ./...; go build -trimpath -ldflags "-s -w -X $(VERSION_PACKAGE)=$(VERSION)" -o /tmp/ipchronicle-center ./cmd/ipchronicle-center; CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w -X $(VERSION_PACKAGE)=$(VERSION)" -o /tmp/ipchronicle-agent-amd64 ./cmd/ipchronicle-agent; CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "-s -w -X $(VERSION_PACKAGE)=$(VERSION)" -o /tmp/ipchronicle-agent-arm64 ./cmd/ipchronicle-agent'
	git diff --exit-code -- go.mod go.sum

check: generate
	./scripts/check-generated.sh
	$(MAKE) web-assets
	$(MAKE) go-check
	./scripts/check-hygiene.sh

build: generate web-assets
	mkdir -p bin
	$(GO_RUN) sh -ceu 'go build -trimpath -ldflags "-s -w -X $(VERSION_PACKAGE)=$(VERSION)" -o bin/ipchronicle-center ./cmd/ipchronicle-center; CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X $(VERSION_PACKAGE)=$(VERSION)" -o bin/ipchronicle-agent ./cmd/ipchronicle-agent'

compose-smoke:
	./scripts/compose-smoke.sh

browser-test:
	PLAYWRIGHT_IMAGE=$(PLAYWRIGHT_IMAGE) ./scripts/browser-test.sh

secret-scan:
	docker run --rm -v $(CURDIR):/repo $(GITLEAKS_IMAGE) dir /repo --redact --no-banner
