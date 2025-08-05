#!/bin/bash

echo "🚀 LLM Balancer Test Setup"
echo "=========================="

# Check if Ollama is running
echo "🔍 Checking Ollama status..."
if ! curl -s http://localhost:11434/api/tags > /dev/null; then
    echo "❌ Ollama is not running on localhost:11434"
    echo "Please start Ollama first:"
    echo "  ollama serve"
    echo ""
    echo "Then pull the qwen2.5 model:"
    echo "  ollama pull qwen2.5"
    exit 1
fi

echo "✅ Ollama is running!"

# Check if qwen2.5 model is available
echo "🔍 Checking for qwen2.5 model..."
if ! curl -s http://localhost:11434/api/tags | grep -q "qwen2.5"; then
    echo "⚠️  qwen2.5 model not found. Pulling it now..."
    ollama pull qwen2.5
    if [ $? -ne 0 ]; then
        echo "❌ Failed to pull qwen2.5 model"
        exit 1
    fi
fi

echo "✅ qwen2.5 model is available!"

# Build the applications
echo "🔨 Building applications..."
go build -o bin/gateway cmd/gateway/main.go
go build -o bin/node cmd/node/main.go
go build -o bin/client cmd/client/main.go

if [ $? -ne 0 ]; then
    echo "❌ Build failed"
    exit 1
fi

echo "✅ Applications built successfully!"

echo ""
echo "📋 Next steps:"
echo "1. Start the gateway:"
echo "   ./bin/gateway config-gateway.yaml"
echo ""
echo "2. In another terminal, start the node:"
echo "   ./bin/node config-test.yaml"
echo ""
echo "3. In another terminal, run the test client:"
echo "   ./bin/client"
echo ""
echo "4. Or test manually with curl:"
echo "   curl -X POST http://localhost:8081/api/v1/tasks \\"
echo "     -H 'Content-Type: application/json' \\"
echo "     -d '{\"payload\": \"Hello, how are you?\", \"parameters\": {\"model\": \"qwen2.5\", \"temperature\": 0.7, \"max_tokens\": 100}, \"priority\": 5}'"
echo ""
echo "5. Check task status:"
echo "   curl http://localhost:8081/api/v1/tasks/{task_id}"
echo ""
echo "6. Check system health:"
echo "   curl http://localhost:8081/api/v1/health"
echo ""
echo "🎯 The system will route tasks to your local Ollama instance and process them with qwen2.5!" 