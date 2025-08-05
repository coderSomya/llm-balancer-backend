# LLM Balancer

A robust distributed load balancer system for LLM instances built in Go. This system provides intelligent task routing, capacity management, and fault tolerance for distributed LLM processing.

## Features

- **Intelligent Load Balancing**: Routes tasks based on node capacity and task complexity
- **Concurrency-Safe Task Queues**: Priority-based task queues with thread-safe operations
- **Capacity Management**: Tracks requests/minute and tokens/minute constraints
- **Health Monitoring**: Real-time node health checking and status monitoring
- **Fault Tolerance**: Circuit breaker patterns and retry mechanisms
- **Gossip Protocol**: Node-to-node communication for failure detection and task redistribution
- **Task Recovery**: Automatic redistribution of tasks from failed nodes
- **Metrics & Observability**: Prometheus metrics and comprehensive monitoring
- **Distributed Architecture**: Support for multiple LLM nodes across machines

## Architecture

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   API Gateway   │    │   Node 1        │    │   Node 2        │
│   (Load Balancer)│    │   - Task Queue  │    │   - Task Queue  │
│   - Route Tasks  │    │   - Workers     │    │   - Workers     │
│   - Health Check │    │   - Capacity    │    │   - Capacity    │
│   - Gossip       │    │   - Gossip      │    │   - Gossip      │
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
│   ├── node/            # Node management & gossip protocol
│   ├── queue/           # Task queue implementation
│   ├── models/          # Data structures
│   ├── config/          # Configuration management
│   └── interfaces/      # Core interfaces
├── pkg/                  # Public packages
│   ├── client/          # Client libraries
│   └── utils/           # Shared utilities
├── config.yaml          # Configuration file
├── Makefile            # Build and run commands
└── README.md           # This file
```

## Core Components

### Load Balancer
- **Multiple strategies**: Round-robin, least-loaded, weighted, random
- **Health-aware routing**: Routes only to healthy nodes
- **Capacity-aware**: Considers node capacity and task complexity
- **Thread-safe**: Concurrent access to node status and capacity

### Gossip Protocol
- **Failure Detection**: Automatic detection of failed nodes
- **Task Redistribution**: Redistribute tasks from failed nodes
- **Peer Discovery**: Automatic peer discovery and management
- **Fault Tolerance**: Three-state failure detection (alive/suspected/dead)

### Task Queue
- **Priority-based**: Higher priority tasks are processed first
- **Concurrency-safe**: Thread-safe operations with mutex protection
- **Capacity management**: Configurable queue size limits
- **Timeout support**: Configurable timeouts for task processing

### Task Tracker
- **Task Monitoring**: Track all tasks and their states
- **Failure Recovery**: Identify failed and orphaned tasks
- **Redistribution**: Mark tasks for redistribution when nodes fail
- **Statistics**: Comprehensive task statistics and monitoring

### Node Management
- **Capacity tracking**: Monitors requests/minute and tokens/minute
- **Health monitoring**: Real-time health status updates
- **Worker pools**: Multithreaded task processing
- **Load balancing**: Intelligent task distribution

## Load Balancing Strategies

1. **Round Robin**: Distributes tasks evenly across nodes
2. **Least Loaded**: Routes to the node with the lowest current load
3. **Weighted**: Considers node capacity, current load, and queue length
4. **Random**: Routes to a random healthy node

## Failure Handling

### Gossip Protocol
- **Periodic Gossip**: Nodes exchange status every 10 seconds
- **Failure Detection**: Nodes marked as suspicious after 15 seconds, dead after 30 seconds
- **Task Redistribution**: Failed node tasks automatically redistributed to healthy nodes
- **Peer Management**: Automatic peer discovery and cleanup

### Task Recovery
- **Failed Tasks**: Tasks that failed or ran too long
- **Orphaned Tasks**: Tasks that have been pending too long
- **Automatic Redistribution**: Tasks redistributed to available nodes
- **State Tracking**: Complete task state tracking for recovery

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

## Quick Start

### 1. Build the applications
```bash
make build
```

### 2. Start the gateway
```bash
make run-gateway
```

### 3. Start multiple nodes (in separate terminals)
```bash
# Terminal 1
NODE_ID=node-1 NODE_PORT=8081 make run-node

# Terminal 2  
NODE_ID=node-2 NODE_PORT=8082 make run-node

# Terminal 3
NODE_ID=node-3 NODE_PORT=8083 make run-node
```

### 4. Submit a task
```bash
curl -X POST http://localhost:8080/api/v1/tasks \
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

## API Endpoints

### Gateway API (`:8080`)

- `POST /api/v1/tasks` - Submit a new task
- `GET /api/v1/tasks/:taskId` - Get task status
- `GET /api/v1/nodes` - List all nodes
- `GET /api/v1/stats` - Get load balancer statistics
- `POST /api/v1/nodes` - Register a new node
- `DELETE /api/v1/nodes/:nodeId` - Unregister a node
- `GET /health` - Health check

### Node API (`:8081`)

- `POST /api/v1/tasks` - Submit a task directly to this node
- `GET /api/v1/tasks/:taskId` - Get task status
- `GET /api/v1/tasks/failed` - Get failed/orphaned tasks for redistribution
- `GET /api/v1/status` - Get node status
- `GET /api/v1/capacity` - Get node capacity
- `GET /api/v1/queue/stats` - Get queue statistics
- `POST /api/v1/gossip` - Handle gossip messages
- `GET /api/v1/peers` - Get known peers
- `GET /health` - Health check

## Development Status

This is Task 1 of the implementation plan:

- ✅ **Core Architecture & Project Structure**
- ✅ **Load Balancer Implementation**
- ✅ **Command-line Applications**
- ✅ **Failure Handling & Gossip Protocol**
- ⏳ Intelligent Routing & Task Classification
- ⏳ Distributed Coordination & Monitoring
- ⏳ Advanced Features & Optimization

## Next Steps

The next task will focus on enhancing the system with:
- Real LLM integration (Ollama)
- Better task classification and routing
- Enhanced metrics and monitoring
- Circuit breaker implementation

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests
5. Submit a pull request

## License

MIT License 