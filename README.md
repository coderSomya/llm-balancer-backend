# LLM Balancer

A robust distributed load balancer system for LLM instances built in Go. This system provides intelligent task routing, capacity management, and fault tolerance for distributed LLM processing.

## Features

- **Intelligent Load Balancing**: Routes tasks based on node capacity and task complexity
- **Concurrency-Safe Task Queues**: Priority-based task queues with thread-safe operations
- **Capacity Management**: Tracks requests/minute and tokens/minute constraints
- **Health Monitoring**: Real-time node health checking and status monitoring
- **Fault Tolerance**: Circuit breaker patterns and retry mechanisms
- **Metrics & Observability**: Prometheus metrics and comprehensive monitoring
- **Distributed Architecture**: Support for multiple LLM nodes across machines

## Architecture

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   API Gateway   │    │   Node 1        │    │   Node 2        │
│   (Load Balancer)│    │   - Task Queue  │    │   - Task Queue  │
│   - Route Tasks  │    │   - Workers     │    │   - Workers     │
│   - Health Check │    │   - Capacity    │    │   - Capacity    │
└─────────────────┘    └─────────────────┘    └─────────────────┘
         │                       │                       │
         └───────────────────────┼───────────────────────┘
                                 │
                    ┌─────────────────┐
                    │   Discovery     │
                    │   Service       │
                    └─────────────────┘
```

## Project Structure

```
llm-balancer/
├── cmd/                    # Application entry points
│   ├── gateway/           # Load balancer API gateway
│   └── node/             # Individual LLM node
├── internal/              # Internal packages
│   ├── balancer/         # Load balancing logic
│   ├── node/            # Node management
│   ├── queue/           # Task queue implementation
│   ├── models/          # Data structures
│   ├── config/          # Configuration management
│   └── interfaces/      # Core interfaces
├── pkg/                  # Public packages
│   ├── client/          # Client libraries
│   └── utils/           # Shared utilities
├── config.yaml          # Configuration file
└── README.md           # This file
```

## Core Components

### Task Queue
- **Priority-based**: Higher priority tasks are processed first
- **Concurrency-safe**: Thread-safe operations with mutex protection
- **Capacity management**: Configurable queue size limits
- **Timeout support**: Configurable timeouts for task processing

### Node Management
- **Capacity tracking**: Monitors requests/minute and tokens/minute
- **Health monitoring**: Real-time health status updates
- **Worker pools**: Multithreaded task processing
- **Load balancing**: Intelligent task distribution

### Load Balancer
- **Multiple strategies**: Round-robin, least-loaded, weighted
- **Health-aware routing**: Routes only to healthy nodes
- **Circuit breaker**: Prevents cascading failures
- **Node discovery**: Automatic node registration/deregistration

## Configuration

The system is configured via `config.yaml`:

```yaml
node:
  id: "node-1"
  address: "localhost:8081"
  max_requests_per_minute: 100
  max_tokens_per_minute: 10000
  max_concurrent_tasks: 10
  max_queue_size: 1000
  worker_pool_size: 5

gateway:
  host: "0.0.0.0"
  port: 8080
  load_balancing_strategy: "round_robin"
  circuit_breaker_enabled: true

queue:
  capacity: 1000
  priority_enabled: true
  timeout: "30s"
  retry_attempts: 3
```

## Usage

### Starting a Node

```bash
go run cmd/node/main.go --config config.yaml
```

### Starting the Gateway

```bash
go run cmd/gateway/main.go --config config.yaml
```

### Submitting Tasks

```bash
curl -X POST http://localhost:8080/tasks \
  -H "Content-Type: application/json" \
  -d '{
    "payload": "Hello, world!",
    "parameters": {
      "model": "llama2",
      "max_tokens": 100
    },
    "priority": 1
  }'
```

## Development Status

This is Task 1 of the implementation plan:

- ✅ **Core Architecture & Project Structure**
- ⏳ Node Implementation
- ⏳ Load Balancer API Gateway
- ⏳ Intelligent Routing & Task Classification
- ⏳ Distributed Coordination & Monitoring
- ⏳ Advanced Features & Optimization

## Next Steps

The next task will focus on implementing the individual LLM nodes with:
- Task queue integration
- Worker pool management
- Capacity tracking
- Health monitoring

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests
5. Submit a pull request

## License

MIT License 