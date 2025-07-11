.PHONY: build test lint fmt test-race test-integration clean help

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

## test: Run all tests with coverage (including integration tests)
test:
	@echo "Running all tests with coverage..."
	@mkdir -p $(COVERAGE_DIR)
	@echo "Running unit tests..."
	@go test -timeout 120s -race -coverprofile=$(COVERAGE_DIR)/coverage.out -covermode=atomic ./...
	@echo "Running integration tests..."
	@go test -timeout 120s -race -tags=integration -coverprofile=$(COVERAGE_DIR)/coverage-integration.out -covermode=atomic ./tests/integration
	@echo "Merging coverage reports..."
	@echo "mode: atomic" > $(COVERAGE_DIR)/coverage-combined.out
	@tail -n +2 $(COVERAGE_DIR)/coverage.out >> $(COVERAGE_DIR)/coverage-combined.out 2>/dev/null || true
	@tail -n +2 $(COVERAGE_DIR)/coverage-integration.out >> $(COVERAGE_DIR)/coverage-combined.out 2>/dev/null || true
	@go tool cover -html=$(COVERAGE_DIR)/coverage-combined.out -o $(COVERAGE_DIR)/coverage.html
	@echo "Coverage report generated at $(COVERAGE_DIR)/coverage.html"

## fmt: Format Go code with gofmt
fmt:
	@echo "Formatting Go code..."
	@gofmt -w .
	@echo "Formatting complete"

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
