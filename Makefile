.PHONY: build test lint test-race test-integration clean help

# Variables
BINARY_NAME := mentat
BINARY_PATH := ./cmd/mentat
BUILD_DIR := ./build
COVERAGE_DIR := ./coverage

# Default target
.DEFAULT_GOAL := help

## help: Show this help message
help:
	@echo "Available targets:"
	@sed -n 's/^##//p' ${MAKEFILE_LIST} | column -t -s ':' | sed -e 's/^/ /'

## build: Build the mentat binary
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	@go build -o $(BUILD_DIR)/$(BINARY_NAME) $(BINARY_PATH)
	@echo "Binary built at $(BUILD_DIR)/$(BINARY_NAME)"

## test: Run all tests with coverage
test:
	@echo "Running tests with coverage..."
	@mkdir -p $(COVERAGE_DIR)
	@go test -race -coverprofile=$(COVERAGE_DIR)/coverage.out -covermode=atomic ./...
	@go tool cover -html=$(COVERAGE_DIR)/coverage.out -o $(COVERAGE_DIR)/coverage.html
	@echo "Coverage report generated at $(COVERAGE_DIR)/coverage.html"

## test-race: Run tests with race detector
test-race:
	@echo "Running tests with race detector..."
	@go test -race ./...

## test-integration: Run integration tests (requires Signal and Claude)
test-integration:
	@echo "Running integration tests..."
	@go test -v -tags=integration ./tests/...

## lint: Run golangci-lint
lint:
	@echo "Running linters..."
	@golangci-lint run

## clean: Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	@rm -rf $(BUILD_DIR) $(COVERAGE_DIR)

## deps: Download dependencies
deps:
	@echo "Downloading dependencies..."
	@go mod download
	@go mod tidy

## run: Run the application
run: build
	@$(BUILD_DIR)/$(BINARY_NAME)