# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
BINARY_NAME=transformer
BINARY_UNIX=$(BINARY_NAME)_unix

# Build targets
.PHONY: all build clean test deps run docker-build docker-run

# Default target
all: test build

# Build the binary
build:
	$(GOBUILD) -o bin/$(BINARY_NAME) -v ./cmd/transformer

# Build for Linux
build-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) -o bin/$(BINARY_UNIX) -v ./cmd/transformer

# Clean build artifacts
clean:
	$(GOCLEAN)
	rm -f bin/$(BINARY_NAME)
	rm -f bin/$(BINARY_UNIX)

# Run tests
test:
	$(GOTEST) -v ./...

# Download dependencies
deps:
	$(GOMOD) download
	$(GOMOD) tidy

# Run the application
run:
	$(GOBUILD) -o bin/$(BINARY_NAME) -v ./cmd/transformer
	./bin/$(BINARY_NAME) serve

# Docker build
docker-build:
	docker build -t transformer-service-go .

# Docker run
docker-run:
	docker run -p 8080:8080 transformer-service-go

# Development run
dev:
	@echo "Running in development mode..."
	$(GOCMD) run ./cmd/transformer serve

# Format code
fmt:
	$(GOCMD) fmt ./...

# Lint code (requires golangci-lint)
lint:
	golangci-lint run

# Security scan (requires gosec)
security:
	gosec ./...