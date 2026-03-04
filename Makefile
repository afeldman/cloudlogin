.PHONY: build run test release test-release clean

VERSION ?= dev
COMMIT := $(shell git rev-parse --short HEAD)
BUILDTIME := $(shell date -u '+%Y-%m-%d %H:%M:%S')

LDFLAGS := -ldflags "-s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(BUILDTIME)"

## build: Build cloudlogin binary
build:
	CGO_ENABLED=1 go build $(LDFLAGS) -o cloudlogin main.go

## run: Build and run cloudlogin
run: build
	./cloudlogin

## clean: Remove binaries and build artifacts
clean:
	rm -f cloudlogin cloudlogin.exe
	rm -rf dist/

## test: Run tests
test:
	go test -v ./...

## deps: Download and tidy dependencies
deps:
	go mod download
	go mod tidy

## release: Create a GitHub release (requires VERSION tag)
release:
	goreleaser release --clean

## test-release: Test release build locally (doesn't publish)
test-release:
	goreleaser release --snapshot --rm-dist

## help: Show this help message
help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@sed -n 's/^## //p' ${MAKEFILE_LIST} | column -t -s ':' | sed -e 's/^/ /'
