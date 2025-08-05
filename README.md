# LLM Load Balancer

A production-ready, distributed load balancer for LLM (Large Language Model) inference with advanced features for high availability, fault tolerance, and intelligent task routing.

## Features

### 🚀 Production-Ready Architecture

- **Distributed Architecture**: Multi-node deployment with automatic node discovery
- **Fault Tolerance**: Circuit breaker patterns, automatic failover, and task redistribution
- **High Availability**: Health checks, automatic recovery, and graceful degradation
- **Scalability**: Horizontal scaling with intelligent load distribution

### 🎯 Advanced Load Balancing

- **Multiple Strategies**: Round-robin, least-loaded, weighted, random, adaptive, and geographic routing
- **Circuit Breakers**: Automatic node isolation on repeated failures
- **Adaptive Routing**: Dynamic strategy selection based on system conditions
- **Priority Queue**: Task prioritization with intelligent scheduling
- **Capacity Management**: Token-based and concurrent task limits

### 📊 Comprehensive Monitoring

- **Real-time Metrics**: Processing times, error rates, queue utilization
- **Health Monitoring**: Node health, queue health, and system status
- **Performance Analytics**: Latency tracking, throughput monitoring
- **Error Tracking**: Detailed error categorization and reporting

### 🔄 Gossip Protocol

- **Node Discovery**: Automatic peer discovery and management
- **Failure Detection**: Advanced failure detection with suspicion and death states
- **Task Redistribution**: Automatic task recovery from failed nodes
- **Latency Monitoring**: Peer latency tracking and optimization
- **Load Sharing**: Intelligent load distribution across healthy nodes

### 🛡️ Robust Error Handling

- **Retry Mechanisms**: Automatic retry with exponential backoff
- **Timeout Management**: Configurable timeouts for all operations
- **Error Recovery**: Graceful handling of network failures and node outages
- **Validation**: Comprehensive input validation and sanitization

### 📈 Advanced Queue Management

- **Priority Queue**: Heap-based priority queue with FIFO for same priority
- **Queue Analytics**: Size distribution, wait times, and utilization metrics
- **Task Tracking**: Complete task lifecycle management
- **Orphaned Task Detection**: Automatic detection and recovery of stuck tasks

## Architecture

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   Gateway       │    │   Node 1        │    │   Node 2        │
│                 │    │                 │    │                 │
│ • Load Balancer │◄──►│ • Task Queue    │    │ • Task Queue    │
│ • Task Registry │    │ • LLM Processor │    │ • LLM Processor │
│ • Health Check  │    │ • Gossip Client │    │ • Gossip Client │
│ • Metrics       │    │ • Circuit Brkr  │    │ • Circuit Brkr  │
└─────────────────┘    └─────────────────┘    └─────────────────┘
         │                       │                       │
         └───────────────────────┼───────────────────────┘
                                 │
                    ┌─────────────────┐
                    │   Gossip        │
                    │   Network       │
                    │                 │
                    │ • Peer Discovery│
                    │ • Failure Detect│
                    │ • Task Redist.  │
                    └─────────────────┘
```

## Quick Start

### Prerequisites

- Go 1.21 or higher
- At least 2GB RAM per node
- Network connectivity between nodes

### Installation

```bash
# Clone the repository
git clone https://github.com/your-org/llm-balancer.git
cd llm-balancer

# Build the binaries
make build

# Or build individually
go build -o bin/gateway cmd/gateway/main.go
go build -o bin/node cmd/node/main.go
```

### Configuration

Create a configuration file `config.yaml`:

```yaml
server:
  host: "0.0.0.0"
  port: 8080

gateway:
  host: "0.0.0.0"
  port: 8081
  load_balancing_strategy: "adaptive"  # round_robin, least_loaded, weighted, random, adaptive, geographic

node:
  id: "node-1"
  address: "localhost:8080"
  max_queue_size: 1000
  max_concurrent_tasks: 10
  max_requests_per_minute: 100
  max_tokens_per_minute: 10000
  worker_pool_size: 5
  heartbeat_interval: "30s"

logging:
  level: "info"
  format: "json"  # json or text
```

### Running the System

1. **Start the Gateway**:
```bash
./bin/gateway config.yaml
```

2. **Start Multiple Nodes**:
```bash
# Node 1
./bin/node config-node1.yaml

# Node 2 (different config)
./bin/node config-node2.yaml
```

3. **Submit Tasks**:
```bash
curl -X POST http://localhost:8081/api/v1/tasks \
  -H "Content-Type: application/json" \
  -d '{
    "payload": "Hello, world!",
    "parameters": {
      "model": "gpt-3.5-turbo",
      "temperature": 0.7,
      "max_tokens": 100
    },
    "priority": 5
  }'
```

## API Reference

### Task Management

#### Submit Task
```http
POST /api/v1/tasks
Content-Type: application/json

{
  "payload": "Your input text",
  "parameters": {
    "model": "gpt-3.5-turbo",
    "temperature": 0.7,
    "max_tokens": 100
  },
  "priority": 5
}
```

#### Get Task Status
```http
GET /api/v1/tasks/{taskId}
```

#### Get All Tasks
```http
GET /api/v1/tasks
```

### Node Management

#### Get Node Status
```http
GET /api/v1/nodes
```

#### Register Node
```http
POST /api/v1/nodes
Content-Type: application/json

    {
      "node_id": "node-1",
  "address": "localhost:8080"
}
```

### System Monitoring

#### Get System Stats
```http
GET /api/v1/stats
```

#### Get Health Status
```http
GET /api/v1/health
```

## Load Balancing Strategies

### Round Robin
- Distributes tasks evenly across all healthy nodes
- Simple and predictable
- Good for uniform workloads

### Least Loaded
- Routes to the node with the lowest current load
- Considers active tasks, queue length, and capacity
- Optimal for maximizing throughput

### Weighted
- Uses sophisticated scoring based on multiple factors:
  - Node capacity (tokens/minute)
  - Current load (active tasks)
  - Queue utilization
  - Health status
- Best for heterogeneous node capabilities

### Random
- Random selection with weighted distribution
- Good for load distribution and avoiding hotspots
- Uses node capacity as weight

### Adaptive
- Dynamically selects strategy based on system conditions:
  - Few nodes (< 3): Least loaded
  - High load (> 80%): Weighted
  - High latency (> 100ms): Geographic
  - Normal conditions: Adaptive weighted random
- Optimal for varying workloads

### Geographic
- Routes based on geographic proximity (placeholder)
- Reduces latency for distributed deployments
- Falls back to weighted selection

## Circuit Breaker Pattern

The system implements circuit breakers for each node:

- **Closed**: Normal operation, requests pass through
- **Open**: Node is failing, requests are blocked
- **Half-Open**: Testing if node has recovered

Configuration:
- Failure threshold: 5 consecutive failures
- Timeout: 30 seconds before retry
- Success threshold: 5 consecutive successes to close

## Gossip Protocol

### Features
- **Peer Discovery**: Automatic node discovery and registration
- **Failure Detection**: Three-state failure detection (alive, suspected, dead)
- **Task Redistribution**: Automatic recovery of tasks from failed nodes
- **Latency Monitoring**: Continuous latency measurement and optimization
- **Load Sharing**: Intelligent load distribution across healthy peers

### States
- **Alive**: Node is healthy and responding
- **Suspected**: Node may be failing, under observation
- **Dead**: Node has failed, tasks being redistributed

## Monitoring and Metrics

### Key Metrics
- **Processing Time**: Average task processing time
- **Error Rate**: Percentage of failed tasks
- **Queue Utilization**: Queue fullness percentage
- **Node Health**: Individual node health status
- **System Load**: Overall system load and capacity

### Health Checks
- **Queue Health**: Queue not too full (< 90%)
- **Capacity Health**: Concurrent tasks within limits
- **Error Rate**: Error rate below threshold (< 10%)
- **Node Connectivity**: All nodes reachable

## Production Deployment

### Docker Deployment

```dockerfile
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o gateway cmd/gateway/main.go
RUN go build -o node cmd/node/main.go

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/gateway .
COPY --from=builder /app/node .
CMD ["./gateway"]
```

### Kubernetes Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: llm-balancer-gateway
spec:
  replicas: 1
  selector:
    matchLabels:
      app: llm-balancer-gateway
  template:
    metadata:
      labels:
        app: llm-balancer-gateway
    spec:
      containers:
      - name: gateway
        image: llm-balancer:latest
        command: ["./gateway"]
        args: ["/config/config.yaml"]
        ports:
        - containerPort: 8081
        volumeMounts:
        - name: config
          mountPath: /config
      volumes:
      - name: config
        configMap:
          name: llm-balancer-config
```

### Environment Variables

```bash
# Gateway configuration
GATEWAY_HOST=0.0.0.0
GATEWAY_PORT=8081
GATEWAY_STRATEGY=adaptive

# Node configuration
NODE_ID=node-1
NODE_ADDRESS=localhost:8080
NODE_MAX_QUEUE_SIZE=1000
NODE_MAX_CONCURRENT_TASKS=10
NODE_WORKER_POOL_SIZE=5

# Logging
LOG_LEVEL=info
LOG_FORMAT=json
```

## Performance Tuning

### Queue Configuration
- **Max Queue Size**: Adjust based on memory and expected load
- **Worker Pool Size**: Match to CPU cores and expected concurrency
- **Priority Levels**: Use 0-10 scale for task prioritization

### Load Balancer Tuning
- **Strategy Selection**: Choose based on workload characteristics
- **Circuit Breaker**: Adjust thresholds for your failure patterns
- **Health Check Interval**: Balance responsiveness vs overhead

### Node Configuration
- **Token Limits**: Set based on your LLM provider limits
- **Concurrent Tasks**: Match to your model's concurrency limits
- **Heartbeat Interval**: Balance responsiveness vs network overhead

## Troubleshooting

### Common Issues

1. **High Error Rate**
   - Check node health and connectivity
   - Verify LLM provider limits
   - Review circuit breaker settings

2. **Queue Full**
   - Increase queue size or add more nodes
   - Check for stuck tasks
   - Review task processing time

3. **Slow Processing**
   - Check node capacity and load
   - Verify LLM provider performance
   - Review network latency

### Debug Commands

```bash
# Check system health
curl http://localhost:8081/api/v1/health

# Get detailed stats
curl http://localhost:8081/api/v1/stats

# Check node status
curl http://localhost:8081/api/v1/nodes

# Get queue stats
curl http://localhost:8080/api/v1/queue/stats
```

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests for new functionality
5. Submit a pull request

## License

This project is licensed under the MIT License - see the LICENSE file for details.

## Support

For support and questions:
- Create an issue on GitHub
- Check the documentation
- Review the troubleshooting guide 