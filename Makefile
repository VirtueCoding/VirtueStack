.PHONY: all build build-controller build-node-agent proto lint test test-integration test-native test-all test-race migrate-up migrate-down certs clean help backup-db

# Build variables
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS := -ldflags "-s -w -X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME)"

# Go commands
GOCMD := go
GOBUILD := $(GOCMD) build $(LDFLAGS)
GOTEST := $(GOCMD) test
GOVET := $(GOCMD) vet
GOMOD := $(GOCMD) mod

TEST_PACKAGES := $(shell $(GOCMD) list ./... | grep -Ev '^github.com/AbuGosok/VirtueStack/(cmd/node-agent|internal/nodeagent($$|/)|internal/shared/libvirtutil$$|tests/integration$$)')
INTEGRATION_TEST_PACKAGES := ./tests/integration
NATIVE_TEST_PACKAGES := $(shell $(GOCMD) list ./... | grep -E '^github.com/AbuGosok/VirtueStack/(cmd/node-agent|internal/nodeagent($$|/)|internal/shared/libvirtutil$$)')

# Output directories
BIN_DIR := bin

# Proto
PROTO_DIR := proto
PROTO_OUT := internal/shared/proto

# Migrations
MIGRATE_DIR := migrations
DATABASE_URL ?= $(or $(DATABASE_URL),postgresql://virtuestack:@localhost:5432/virtuestack?sslmode=disable)  # local dev only - not for production

## help: Show this help message
help:
	@echo "VirtueStack Makefile targets:"
	@echo ""
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## /  /'

## all: Build all binaries
all: build

## build: Build controller and node-agent binaries
build: build-controller build-node-agent

## build-controller: Build the Controller binary
build-controller:
	$(GOBUILD) -o $(BIN_DIR)/controller ./cmd/controller

## build-node-agent: Build the Node Agent binary
build-node-agent:
	$(GOBUILD) -o $(BIN_DIR)/node-agent ./cmd/node-agent

## proto: Generate Go code from protobuf definitions
proto:
	protoc \
		--go_out=$(PROTO_OUT) --go_opt=paths=source_relative \
		--go-grpc_out=$(PROTO_OUT) --go-grpc_opt=paths=source_relative \
		-I $(PROTO_DIR) \
		$(PROTO_DIR)/virtuestack/*.proto

## lint: Run golangci-lint
lint:
	golangci-lint run ./...

## vet: Run go vet
vet:
	$(GOVET) ./...

## test: Run Go tests that do not require native libvirt/Ceph headers
test:
	$(GOTEST) $(TEST_PACKAGES)

## test-native: Run native node-agent tests (requires libvirt/Ceph development headers)
test-native:
	$(GOTEST) $(NATIVE_TEST_PACKAGES)

## test-integration: Run integration tests (requires Docker/Testcontainers)
test-integration:
	$(GOTEST) $(INTEGRATION_TEST_PACKAGES)

## test-all: Run the default, integration, and native node-agent test suites
test-all: test test-integration test-native

## test-race: Run non-native tests with race detector
test-race:
	$(GOTEST) -race -count=1 $(TEST_PACKAGES)

## test-coverage: Run tests with coverage report
test-coverage:
	$(GOTEST) -race -coverprofile=coverage.out -covermode=atomic $(TEST_PACKAGES)
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

## migrate-up: Run database migrations
migrate-up:
	migrate -path $(MIGRATE_DIR) -database "$(DATABASE_URL)" up

## migrate-down: Rollback last database migration
migrate-down:
	migrate -path $(MIGRATE_DIR) -database "$(DATABASE_URL)" down 1

## migrate-create: Create a new migration (usage: make migrate-create NAME=create_users)
migrate-create:
	migrate create -ext sql -dir $(MIGRATE_DIR) -seq $(NAME)

## certs: Generate mTLS certificates for development
certs:
	./scripts/certs/generate.sh

## deps: Download and verify dependencies
deps:
	$(GOMOD) download
	$(GOMOD) verify
	$(GOMOD) tidy

## vuln: Run Go vulnerability check
vuln:
	govulncheck ./...

## npm-audit: Run npm audit for all frontends
npm-audit:
	@echo "Auditing admin webui dependencies..."
	@cd webui/admin && npm audit --audit-level=moderate
	@echo "Auditing customer webui dependencies..."
	@cd webui/customer && npm audit --audit-level=moderate

## clean: Remove build artifacts
clean:
	rm -rf $(BIN_DIR) coverage.out coverage.html

## backup-db: Create an encrypted database backup (requires DATABASE_URL and ENCRYPTION_KEY)
backup-db:
	./scripts/backup-config.sh

## docker-build: Build Docker images
docker-build:
	docker compose build

## docker-up: Start all services
docker-up:
	docker compose up -d

## docker-down: Stop all services
docker-down:
	docker compose down
