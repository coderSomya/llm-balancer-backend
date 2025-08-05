.PHONY: build clean test run-gateway run-node run-client setup

# Build all components
build:
	@echo "🔨 Building LLM Balancer components..."
	@mkdir -p bin
	go build -o bin/gateway cmd/gateway/main.go
	go build -o bin/node cmd/node/main.go
	go build -o bin/client cmd/client/main.go
	@echo "✅ Build complete!"

# Clean build artifacts
clean:
	@echo "🧹 Cleaning build artifacts..."
	rm -rf bin/
	@echo "✅ Clean complete!"

# Run setup script
setup:
	@echo "🚀 Running setup script..."
	./test-setup.sh

# Run gateway
run-gateway:
	@echo "🌐 Starting Gateway..."
	./bin/gateway config-gateway.yaml

# Run node
run-node:
	@echo "🖥️  Starting Node..."
	./bin/node config-test.yaml

# Run client
run-client:
	@echo "📤 Running Test Client..."
	./bin/client

# Test the complete system
test-system:
	@echo "🧪 Testing complete system..."
	@echo "1. Make sure Ollama is running: ollama serve"
	@echo "2. Make sure qwen2.5 is available: ollama pull qwen2.5"
	@echo "3. Start gateway in one terminal: make run-gateway"
	@echo "4. Start node in another terminal: make run-node"
	@echo "5. Run client in third terminal: make run-client"

# Check if Ollama is running
check-ollama:
	@echo "🔍 Checking Ollama status..."
	@if curl -s http://localhost:11434/api/tags > /dev/null; then \
		echo "✅ Ollama is running"; \
	else \
		echo "❌ Ollama is not running. Please start it with: ollama serve"; \
		exit 1; \
	fi

# Check if qwen2.5 model is available
check-model:
	@echo "🔍 Checking for qwen2.5 model..."
	@if curl -s http://localhost:11434/api/tags | grep -q "qwen2.5"; then \
		echo "✅ qwen2.5 model is available"; \
	else \
		echo "⚠️  qwen2.5 model not found. Pulling it now..."; \
		ollama pull qwen2.5; \
	fi

# Full setup and test
all: check-ollama check-model build test-system

# Help
help:
	@echo "LLM Balancer Makefile Commands:"
	@echo ""
	@echo "  build        - Build all components (gateway, node, client)"
	@echo "  clean        - Remove build artifacts"
	@echo "  setup        - Run the setup script"
	@echo "  run-gateway  - Start the gateway server"
	@echo "  run-node     - Start a node server"
	@echo "  run-client   - Run the test client"
	@echo "  test-system  - Show instructions for testing the complete system"
	@echo "  check-ollama - Check if Ollama is running"
	@echo "  check-model  - Check if qwen2.5 model is available"
	@echo "  all          - Full setup: check Ollama, pull model, build, test"
	@echo "  help         - Show this help message"
	@echo ""
	@echo "Quick start:"
	@echo "  1. make check-ollama"
	@echo "  2. make check-model"
	@echo "  3. make build"
	@echo "  4. make run-gateway (in terminal 1)"
	@echo "  5. make run-node (in terminal 2)"
	@echo "  6. make run-client (in terminal 3)" 