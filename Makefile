.PHONY: build run-gateway run-node test clean help

# Default target
all: build

# Build the applications
build:
	@echo "Building LLM Balancer..."
	go build -o bin/gateway cmd/gateway/main.go
	go build -o bin/node cmd/node/main.go
	@echo "Build complete!"

# Run the gateway
run-gateway: build
	@echo "Starting Gateway..."
	./bin/gateway config.yaml

# Run a node
run-node: build
	@echo "Starting Node..."
	./bin/node config.yaml

# Run both gateway and node (in separate terminals)
run-all: build
	@echo "Starting Gateway and Node..."
	@echo "Gateway will run on port 8080"
	@echo "Node will run on port 8081"
	@echo "Use 'make run-gateway' and 'make run-node' in separate terminals"

# Test the applications
test:
	@echo "Running tests..."
	go test ./...

# Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -rf bin/
	go clean

# Install dependencies
deps:
	@echo "Installing dependencies..."
	go mod download
	go mod tidy

# Format code
fmt:
	@echo "Formatting code..."
	go fmt ./...

# Lint code
lint:
	@echo "Linting code..."
	golangci-lint run

# Show help
help:
	@echo "LLM Balancer Makefile"
	@echo ""
	@echo "Available targets:"
	@echo "  build       - Build the applications"
	@echo "  run-gateway - Run the gateway server"
	@echo "  run-node    - Run a node server"
	@echo "  run-all     - Show instructions for running both"
	@echo "  test        - Run tests"
	@echo "  clean       - Clean build artifacts"
	@echo "  deps        - Install dependencies"
	@echo "  fmt         - Format code"
	@echo "  lint        - Lint code"
	@echo "  help        - Show this help" 