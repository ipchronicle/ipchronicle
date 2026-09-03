SHELL := /bin/bash

include scripts/release-images.env

CONTAINER_USER := $(shell id -u):$(shell id -g)
ROOT := /workspace
VERSION ?= dev
REVISION ?= $(shell git rev-parse HEAD 2>/dev/null || printf unknown)
VERSION_PACKAGE := github.com/ipchronicle/ipchronicle/internal/version.Value
REVISION_PACKAGE := github.com/ipchronicle/ipchronicle/internal/version.Revision
GO_LDFLAGS := -s -w -buildid= -extldflags=-Wl,--build-id=none -X $(VERSION_PACKAGE)=$(VERSION) -X $(REVISION_PACKAGE)=$(REVISION)
CENTER_GOEXPERIMENT := nogreenteagc
GO_RUN := docker run --rm --user $(CONTAINER_USER) -e HOME=/tmp -e GOCACHE=/tmp/go-build -e GOMODCACHE=/tmp/go-mod -v $(CURDIR):$(ROOT) -w $(ROOT) $(GO_IMAGE)
NODE_RUN := docker run --rm --user $(CONTAINER_USER) -e HOME=/tmp -v $(CURDIR):$(ROOT) -w $(ROOT)/web $(NODE_IMAGE)
SQLC_RUN := docker run --rm --user $(CONTAINER_USER) -v $(CURDIR):/src -w /src $(SQLC_IMAGE)

.PHONY: all browser-test build check compose-smoke format generate go-check go-preflight preflight release-candidate release-failure-gate release-version-check secret-scan verify-release-candidate web-assets web-check web-preflight

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

web-preflight:
	$(NODE_RUN) sh -ceu 'npm ci; npm run preflight'

web-assets: web-check
	./scripts/sync-web-assets.sh

go-check:
	$(GO_RUN) sh -ceu 'go mod tidy; unformatted=$$(gofmt -l $$(find . -type f -name "*.go" -not -path "./web/node_modules/*")); test -z "$$unformatted" || { printf "Unformatted Go files:\n%s\n" "$$unformatted"; exit 1; }; GOEXPERIMENT=$(CENTER_GOEXPERIMENT) go vet ./...; GOEXPERIMENT=$(CENTER_GOEXPERIMENT) go test ./...; GOEXPERIMENT=$(CENTER_GOEXPERIMENT) go test -race ./...; GOEXPERIMENT=$(CENTER_GOEXPERIMENT) go build -trimpath -buildvcs=false -ldflags "$(GO_LDFLAGS)" -o /tmp/ipchronicle-center ./cmd/ipchronicle-center; go version -m /tmp/ipchronicle-center | grep -F "GOEXPERIMENT=$(CENTER_GOEXPERIMENT)"; CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -buildvcs=false -ldflags "$(GO_LDFLAGS)" -o /tmp/ipchronicle-agent-amd64 ./cmd/ipchronicle-agent; CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -buildvcs=false -ldflags "$(GO_LDFLAGS)" -o /tmp/ipchronicle-agent-arm64 ./cmd/ipchronicle-agent'
	git diff --exit-code -- go.mod go.sum

go-preflight:
	$(GO_RUN) sh -ceu 'go mod download; go mod tidy; unformatted=$$(gofmt -l $$(find . -type f -name "*.go" -not -path "./web/node_modules/*")); test -z "$$unformatted" || { printf "Unformatted Go files:\n%s\n" "$$unformatted"; exit 1; }; GOEXPERIMENT=$(CENTER_GOEXPERIMENT) go vet ./...; GOEXPERIMENT=$(CENTER_GOEXPERIMENT) go test ./...'
	git diff --exit-code -- go.mod go.sum

check: release-version-check generate
	./scripts/check-generated.sh
	$(MAKE) web-assets
	$(MAKE) go-check
	./scripts/check-hygiene.sh

preflight: release-version-check
	./scripts/check-generated.sh
	$(MAKE) web-preflight
	$(MAKE) go-preflight
	./scripts/check-hygiene.sh

build: generate web-assets
	mkdir -p bin
	$(GO_RUN) sh -ceu 'GOEXPERIMENT=$(CENTER_GOEXPERIMENT) go build -trimpath -buildvcs=false -ldflags "$(GO_LDFLAGS)" -o bin/ipchronicle-center ./cmd/ipchronicle-center; CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags "$(GO_LDFLAGS)" -o bin/ipchronicle-agent ./cmd/ipchronicle-agent'

compose-smoke:
	./scripts/compose-smoke.sh

browser-test:
	PLAYWRIGHT_IMAGE=$(PLAYWRIGHT_IMAGE) ./scripts/browser-test.sh

release-candidate:
	./scripts/build-release-candidate.sh "$(VERSION)"

release-version-check:
	./scripts/check-release-version.sh

verify-release-candidate:
	./scripts/verify-release-candidate.sh "dist/release/$(VERSION)"

release-failure-gate:
	./scripts/test-release-failures.sh

secret-scan:
	docker run --rm -v $(CURDIR):/repo $(GITLEAKS_IMAGE) dir /repo --redact --no-banner
