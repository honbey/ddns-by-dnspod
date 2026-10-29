# Local GOPATH is ~/.go, not the default ~/go
GOPATH ?= $(HOME)/.go
# Domestic module proxy by default; override for other networks.
GOPROXY ?= https://goproxy.cn,direct
export GOPATH
export GOPROXY

BINARY := bin/ddns-server
BACKEND_DIR := backend
FRONTEND_DIR := frontend
# The version is written in one place: info.version in backend/openapi.yml.
# frontend/package.json carries the same number and CI checks they agree.
VERSION ?= $(shell sed -n 's/^[[:space:]]*version: *"\([^"]*\)".*/\1/p' $(BACKEND_DIR)/openapi.yml)

.PHONY: all help deps frontend backend build run version dev-frontend dev-backend dev-idp types test test-e2e tidy fmt vet clean

all: build

help:
	@echo "Targets:"
	@echo "  make deps         install backend and frontend dependencies"
	@echo "  make frontend     build the frontend (output: backend/internal/frontend/dist)"
	@echo "  make backend      compile the backend binary"
	@echo "  make build        frontend then backend (single executable)"
	@echo "  make version      print the version the build stamps in"
	@echo "  make run          build, then run the binary with config.local.yml"
	@echo "  make dev-frontend start the frontend dev server (5173), API proxied to 8080"
	@echo "  make dev-backend  run the backend in the foreground"
	@echo "  make dev-idp      start a local OpenID Connect provider (9443) for testing"
	@echo "  make types        regenerate the SPA types from backend/openapi.yml"
	@echo "  make test         run the backend and frontend test suites"
	@echo "  make test-e2e     run the browser and backend end-to-end suites"
	@echo "  make tidy         tidy go.mod"
	@echo "  make fmt          format Go code"
	@echo "  make vet          static checks"
	@echo "  make clean        remove build output"

deps:
	cd $(BACKEND_DIR) && go mod download
	cd $(FRONTEND_DIR) && npm install

frontend:
	cd $(FRONTEND_DIR) && npm run build

# The SPA is embedded at compile time, so the backend cannot be built before it
# - a dependency, not just an ordering in `build` that `make -j` could ignore.
backend: frontend
	cd $(BACKEND_DIR) && go build -ldflags="-s -w -X ddns-by-dnspod/internal/config.Version=$(VERSION)" -o ../$(BINARY) ./cmd/server

version:
	@echo $(VERSION)

build: frontend backend
	@echo "Build complete: $(BINARY)"

run: build
	./$(BINARY) -c config.local.yml

dev-backend:
	cd $(BACKEND_DIR) && go run ./cmd/server

dev-frontend:
	cd $(FRONTEND_DIR) && npm run dev

# An explicit refresh, not a build prerequisite: `prebuild` and `pretypecheck`
# already regenerate the types, so a build from a clean checkout is fine either
# way. Run this after editing backend/openapi.yml when you want to see the
# generated diff before committing it.
types:
	@test -d $(FRONTEND_DIR)/node_modules || { echo "frontend/node_modules is missing: run 'make deps' first"; exit 1; }
	cd $(FRONTEND_DIR) && npm run types

tidy:
	cd $(BACKEND_DIR) && go mod tidy

fmt:
	cd $(BACKEND_DIR) && go fmt ./...

vet:
	cd $(BACKEND_DIR) && go vet ./...
	cd $(BACKEND_DIR) && go vet -tags=e2e ./e2e/

# Both suites: the frontend is embedded in the binary, so a change there is as
# much a part of the server as the Go code is. --run because vitest otherwise
# stays in watch mode outside a terminal.
test:
	cd $(BACKEND_DIR) && go test ./...
	cd $(FRONTEND_DIR) && npm test -- --run

# Playwright drives the real binary, so the SPA has to be embedded first.
test-e2e: frontend
	cd $(BACKEND_DIR) && go test -count=1 -tags=e2e ./e2e/ -v
	cd $(FRONTEND_DIR) && npm run test:e2e

clean:
	rm -rf bin backend/internal/frontend/dist $(FRONTEND_DIR)/test-results $(FRONTEND_DIR)/playwright-report

dev-idp:
	CLIENT_ID=ddns-by-dnspod node $(FRONTEND_DIR)/e2e/mock-idp.mjs

