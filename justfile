# E-Hentai Database - Justfile

# Show all available commands
default:
    @just --list

# Install dependencies
deps:
    go mod download
    go mod tidy

# Update all dependencies
deps-update:
    go get -u ./...
    go mod tidy

# Generate module dependency graph
deps-graph:
    go mod graph | grep github.com/slinet/ehdb

# Check outdated dependencies
deps-check:
    go list -u -m all

# Build all binaries
build: build-api build-sync

# Build API server
build-api:
    go build -o bin/ehdb-api cmd/api/main.go

# Build sync tool
build-sync:
    go build -o bin/ehdb-sync cmd/sync/main.go

# Clean build artifacts
clean:
    rm -rf bin/

# Run API server
run-api:
    go run cmd/api/main.go -config config.yaml

# Run API server (with scheduler)
run-api-scheduler:
    go run cmd/api/main.go -config config.yaml -scheduler

# Sync latest galleries
sync host="" offset="0":
	#!/usr/bin/env bash
	cmd="go run cmd/sync/main.go sync -config config.yaml"
	[ -n "{{host}}" ] && cmd="$cmd -host {{host}}"
	[ "{{offset}}" != "0" ] && cmd="$cmd -offset {{offset}}"
	eval $cmd

# Resync galleries from last N hours
resync hours="24":
	go run cmd/sync/main.go resync -config config.yaml -hours {{hours}}

# Fetch specified galleries manually
fetch +gids:
	go run cmd/sync/main.go fetch -config config.yaml {{gids}}

# Fetch galleries from file
fetch-file file:
	go run cmd/sync/main.go fetch -config config.yaml -file {{file}}

# Sync torrents
torrent-sync host="":
	#!/usr/bin/env bash
	cmd="go run cmd/sync/main.go torrent-sync -config config.yaml"
	[ -n "{{host}}" ] && cmd="$cmd -host {{host}}"
	eval $cmd

# Import all galleries' torrents (use with caution)
torrent-import host="":
	#!/usr/bin/env bash
	echo "WARNING: This is a heavy operation that will scan all galleries"
	echo "Press Ctrl+C to cancel, or press Enter to continue..."
	read
	cmd="go run cmd/sync/main.go torrent-import -config config.yaml"
	[ -n "{{host}}" ] && cmd="$cmd -host {{host}}"
	eval $cmd

# Mark replaced galleries
mark-replaced:
	go run cmd/sync/main.go mark-replaced -config config.yaml

# Run all tests
test:
    go test -v ./...

# Run tests (with coverage)
test-coverage:
    go test -v -coverprofile=coverage.out ./...
    go tool cover -html=coverage.out

# Format code
fmt:
    go fmt ./...

# Code check
vet:
    go vet ./...

# Run linter (need to install golangci-lint)
lint:
    golangci-lint run

# Create config file
init-config:
    @if [ ! -f config.yaml ]; then \
        cp config.example.yaml config.yaml; \
        echo "config.yaml has been created, please edit the configuration"; \
    else \
        echo "config.yaml already exists"; \
    fi

# Production environment build (build + clean)
release: clean build
    @echo "Build completed, binary files in bin/ directory"

# Full CI check (format + check + test)
ci: fmt vet test
    @echo "CI check passed"
