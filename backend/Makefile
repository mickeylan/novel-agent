.PHONY: build run test clean deps

# Binary name
BINARY_NAME=novel-agent

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOMOD=$(GOCMD) mod
GOFMT=gofmt

# Build the binary
build:
	$(GOBUILD) -o $(BINARY_NAME) ./cmd/server

# Run the application
run:
	$(GOCMD) run ./cmd/server

# Run tests
test:
	$(GOTEST) -v ./...

# Download dependencies
deps:
	$(GOMOD) download
	$(GOMOD) tidy

# Format code
fmt:
	$(GOFMT) -s -w .

# Clean build artifacts
clean:
	rm -f $(BINARY_NAME)
	rm -f $(BINARY_NAME).exe

# Database migrations are intentionally disabled: this service reuses an existing database.

# Development server with hot reload (requires air)
dev:
	air ./cmd/server

# Build for Linux
build-linux:
	GOOS=linux GOARCH=amd64 $(GOBUILD) -o $(BINARY_NAME)-linux ./cmd/server

# Build for Windows
build-windows:
	GOOS=windows GOARCH=amd64 $(GOBUILD) -o $(BINARY_NAME).exe ./cmd/server

# Lint code
lint:
	golangci-lint run ./...

# Show help
help:
	@echo "Available targets:"
	@echo "  build        - Build the binary"
	@echo "  run          - Run the application"
	@echo "  test         - Run tests"
	@echo "  deps         - Download dependencies"
	@echo "  fmt          - Format code"
	@echo "  clean        - Clean build artifacts"
	@echo "  dev          - Run with hot reload"
	@echo "  build-linux  - Build for Linux"
	@echo "  build-windows - Build for Windows"
	@echo "  lint         - Run linter"
