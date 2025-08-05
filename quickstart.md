# LLM Balancer Quick Start

This guide will help you quickly test the LLM balancer with your local Ollama instance and qwen2.5 model.

## Prerequisites

1. **Ollama installed and running**
   ```bash
   # Install Ollama (if not already installed)
   curl -fsSL https://ollama.ai/install.sh | sh
   
   # Start Ollama
   ollama serve
   ```

2. **qwen2.5 model pulled**
   ```bash
   ollama pull qwen2.5
   ```

3. **Go 1.21+ installed**

## Quick Test (3 Steps)

### Step 1: Build Everything
```bash
make build
```

### Step 2: Start the Services (in separate terminals)

**Terminal 1 - Gateway:**
```bash
make run-gateway
```

**Terminal 2 - Node:**
```bash
make run-node
```

### Step 3: Test the System

**Terminal 3 - Client:**
```bash
make run-client
```

## What You'll See

### Gateway Output:
```
Starting gateway server on 0.0.0.0:8081
```

### Node Output:
```
Ollama connection established
Available models: [qwen2.5]
Starting node server on 0.0.0.0:8080
```

### Client Output:
```
LLM Balancer Test Client
==========================
Connecting to gateway at: http://localhost:8081
Gateway is healthy!

Submitting task 1: Hello, how are you today?
Task submitted successfully. Task ID: task-1234567890

Submitting task 2: What is the capital of France?
Task submitted successfully. Task ID: task-1234567891

Monitoring task completion...
Checking task status (attempt 1/30)...
Task task-1234567890 completed!
Result: Hello! I'm doing well, thank you for asking...

Task task-1234567891 completed!
Result: The capital of France is Paris...

All 5 tasks completed successfully!
```

## Manual Testing

### Submit a Task
```bash
curl -X POST http://localhost:8081/api/v1/tasks \
  -H 'Content-Type: application/json' \
  -d '{
    "payload": "Hello, how are you today?",
    "parameters": {
      "model": "qwen2.5",
      "temperature": 0.7,
      "max_tokens": 100
    },
    "priority": 5
  }'
```

### Check Task Status
```bash
# Replace {task_id} with the actual task ID from the response
curl http://localhost:8081/api/v1/tasks/{task_id}
```

### Check System Health
```bash
curl http://localhost:8081/api/v1/health
```

## Troubleshooting

### Ollama Issues
```bash
# Check if Ollama is running
make check-ollama

# Check if qwen2.5 is available
make check-model
```

### Build Issues
```bash
# Clean and rebuild
make clean
make build
```

### Port Conflicts
- Gateway: 8081
- Node: 8080  
- Ollama: 11434

Make sure these ports are available.

## Monitoring

### Gateway Metrics
```bash
curl http://localhost:8081/api/v1/stats
```

### Node Metrics
```bash
curl http://localhost:8080/api/v1/metrics
```

### Queue Status
```bash
curl http://localhost:8080/api/v1/queue/stats
```

## Architecture

```
Client → Gateway (8081) → Node (8080) → Ollama (11434)
   ↑         ↓              ↓
   └─────────┴──────────────┘
        Status Polling
```

##  What's Working

-  **Task Routing**: Gateway routes tasks to nodes
-  **Queue Management**: Tasks are queued and processed
-  **Real LLM Integration**: Actual qwen2.5 responses
-  **Load Balancing**: Intelligent task distribution
-  **Health Monitoring**: System health checks
-  **Error Handling**: Graceful error recovery
-  **Metrics Collection**: Performance monitoring

##  Help

```bash
# Show all available commands
make help

# Full setup with checks
make all
```

---

** Congratulations!** You now have a working LLM load balancer that integrates with your local Ollama instance and processes real AI requests with qwen2.5! 