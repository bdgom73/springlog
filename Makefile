BINARY   := springlog
VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS  := -ldflags "-X springlog/cli.Version=$(VERSION) -s -w"

.PHONY: build build-all test lint clean install

## build: Build for current OS
build:
	go build $(LDFLAGS) -o $(BINARY) ./cmd/springlog

## build-all: Cross-compile for Windows, macOS, Linux
build-all:
	mkdir -p dist
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o dist/$(BINARY)-$(VERSION)-windows-amd64.exe ./cmd/springlog
	GOOS=darwin  GOARCH=amd64 go build $(LDFLAGS) -o dist/$(BINARY)-$(VERSION)-darwin-amd64  ./cmd/springlog
	GOOS=darwin  GOARCH=arm64 go build $(LDFLAGS) -o dist/$(BINARY)-$(VERSION)-darwin-arm64  ./cmd/springlog
	GOOS=linux   GOARCH=amd64 go build $(LDFLAGS) -o dist/$(BINARY)-$(VERSION)-linux-amd64   ./cmd/springlog
	GOOS=linux   GOARCH=arm64 go build $(LDFLAGS) -o dist/$(BINARY)-$(VERSION)-linux-arm64   ./cmd/springlog

## test: Run unit tests
test:
	go test ./... -v -race

## lint: Run golangci-lint
lint:
	golangci-lint run ./...

## clean: Remove build artifacts
clean:
	rm -f $(BINARY) $(BINARY).exe
	rm -rf dist/

## install: Install to GOPATH/bin
install:
	go install $(LDFLAGS) ./cmd/springlog

help:
	@grep -E '^##' Makefile | sed 's/## //'
