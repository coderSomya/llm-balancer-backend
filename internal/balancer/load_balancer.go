package balancer

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"
	"llm-balancer/internal/models"
)

// CircuitBreakerState represents the state of a circuit breaker
type CircuitBreakerState int

const (
	CircuitBreakerClosed CircuitBreakerState = iota
	CircuitBreakerOpen
	CircuitBreakerHalfOpen
)

// CircuitBreaker tracks failure rates and manages node availability
type CircuitBreaker struct {
	state           CircuitBreakerState
	failureCount    int
	lastFailureTime time.Time
	successCount    int
	threshold       int
	timeout         time.Duration
	mu              sync.RWMutex
}

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(threshold int, timeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		state:     CircuitBreakerClosed,
		threshold: threshold,
		timeout:   timeout,
	}
}

// RecordSuccess records a successful operation
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	
	cb.successCount++
	if cb.state == CircuitBreakerHalfOpen && cb.successCount >= cb.threshold {
		cb.state = CircuitBreakerClosed
		cb.failureCount = 0
		cb.successCount = 0
	}
}

// RecordFailure records a failed operation
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	
	cb.failureCount++
	cb.lastFailureTime = time.Now()
	
	if cb.failureCount >= cb.threshold {
		cb.state = CircuitBreakerOpen
		cb.successCount = 0
	}
}

// IsAvailable checks if the circuit breaker allows operations
func (cb *CircuitBreaker) IsAvailable() bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	
	switch cb.state {
	case CircuitBreakerClosed:
		return true
	case CircuitBreakerOpen:
		// Check if timeout has passed
		if time.Since(cb.lastFailureTime) > cb.timeout {
			cb.mu.RUnlock()
			cb.mu.Lock()
			cb.state = CircuitBreakerHalfOpen
			cb.mu.Unlock()
			cb.mu.RLock()
			return true
		}
		return false
	case CircuitBreakerHalfOpen:
		return true
	default:
		return false
	}
}

// LoadBalancer implements intelligent task routing with advanced features
type LoadBalancer struct {
	nodes           map[string]*models.NodeStatus
	capacities      map[string]*models.NodeCapacity
	circuitBreakers map[string]*CircuitBreaker
	strategy        string
	mu              sync.RWMutex
	lastIndex       int
	rand            *rand.Rand
	
	// Advanced features
	healthCheckInterval time.Duration
	metrics            map[string]interface{}
	adaptiveStrategy   bool
	lastStrategyChange time.Time
}

// NewLoadBalancer creates a new load balancer with advanced features
func NewLoadBalancer(strategy string) *LoadBalancer {
	return &LoadBalancer{
		nodes:              make(map[string]*models.NodeStatus),
		capacities:         make(map[string]*models.NodeCapacity),
		circuitBreakers:    make(map[string]*CircuitBreaker),
		strategy:           strategy,
		rand:               rand.New(rand.NewSource(time.Now().UnixNano())),
		healthCheckInterval: 30 * time.Second,
		metrics:            make(map[string]interface{}),
		adaptiveStrategy:   true,
	}
}

// RouteTask routes a task to the best available node with advanced selection
func (lb *LoadBalancer) RouteTask(ctx context.Context, task *models.Task) (string, error) {
	lb.mu.RLock()
	defer lb.mu.RUnlock()
	
	// Get healthy nodes with circuit breaker checks
	healthyNodes := lb.getHealthyNodes()
	if len(healthyNodes) == 0 {
		return "", fmt.Errorf("no healthy nodes available")
	}
	
	// Update metrics
	lb.updateRoutingMetrics(task, len(healthyNodes))
	
	// Route based on strategy with adaptive switching
	strategy := lb.getOptimalStrategy(healthyNodes, task)
	
	switch strategy {
	case "round_robin":
		return lb.roundRobin(healthyNodes)
	case "least_loaded":
		return lb.leastLoaded(healthyNodes, task)
	case "weighted":
		return lb.weighted(healthyNodes, task)
	case "random":
		return lb.random(healthyNodes)
	case "adaptive":
		return lb.adaptive(healthyNodes, task)
	case "geographic":
		return lb.geographic(healthyNodes, task)
	default:
		return lb.roundRobin(healthyNodes)
	}
}

// getOptimalStrategy determines the best strategy based on current conditions
func (lb *LoadBalancer) getOptimalStrategy(healthyNodes []string, task *models.Task) string {
	if !lb.adaptiveStrategy {
		return lb.strategy
	}
	
	// Analyze current conditions
	nodeCount := len(healthyNodes)
	totalLoad := lb.calculateTotalLoad(healthyNodes)
	avgLatency := lb.calculateAverageLatency(healthyNodes)
	
	// Adaptive strategy selection
	if nodeCount < 3 {
		return "least_loaded" // Few nodes, prioritize load distribution
	} else if totalLoad > 0.8 {
		return "weighted" // High load, use sophisticated balancing
	} else if avgLatency > 100*time.Millisecond {
		return "geographic" // High latency, consider location
	} else {
		return "adaptive" // Normal conditions, use adaptive
	}
}

// calculateTotalLoad calculates the total load across all nodes
func (lb *LoadBalancer) calculateTotalLoad(healthyNodes []string) float64 {
	var totalLoad float64
	for _, nodeID := range healthyNodes {
		if status := lb.nodes[nodeID]; status != nil {
			totalLoad += status.GetLoad()
		}
	}
	return totalLoad / float64(len(healthyNodes))
}

// calculateAverageLatency calculates average latency (placeholder for real implementation)
func (lb *LoadBalancer) calculateAverageLatency(healthyNodes []string) time.Duration {
	// In a real implementation, this would query actual latency metrics
	return 50 * time.Millisecond
}

// updateRoutingMetrics updates routing metrics for monitoring
func (lb *LoadBalancer) updateRoutingMetrics(task *models.Task, availableNodes int) {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	
	// Update task routing metrics
	if metrics, exists := lb.metrics["task_routing"].(map[string]interface{}); exists {
		if count, ok := metrics["total_tasks"].(int); ok {
			metrics["total_tasks"] = count + 1
		}
		metrics["available_nodes"] = availableNodes
		metrics["last_routing_time"] = time.Now()
	} else {
		lb.metrics["task_routing"] = map[string]interface{}{
			"total_tasks":      1,
			"available_nodes":   availableNodes,
			"last_routing_time": time.Now(),
		}
	}
}

// RegisterNode registers a new node with circuit breaker
func (lb *LoadBalancer) RegisterNode(nodeID string, status *models.NodeStatus) error {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	
	lb.nodes[nodeID] = status
	lb.circuitBreakers[nodeID] = NewCircuitBreaker(5, 30*time.Second)
	return nil
}

// UnregisterNode unregisters a node
func (lb *LoadBalancer) UnregisterNode(nodeID string) error {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	
	delete(lb.nodes, nodeID)
	delete(lb.capacities, nodeID)
	delete(lb.circuitBreakers, nodeID)
	return nil
}

// UpdateNodeStatus updates the status of a node
func (lb *LoadBalancer) UpdateNodeStatus(nodeID string, status *models.NodeStatus) error {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	
	lb.nodes[nodeID] = status
	return nil
}

// UpdateNodeCapacity updates the capacity of a node
func (lb *LoadBalancer) UpdateNodeCapacity(nodeID string, capacity *models.NodeCapacity) error {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	
	lb.capacities[nodeID] = capacity
	return nil
}

// RecordNodeSuccess records a successful operation for a node
func (lb *LoadBalancer) RecordNodeSuccess(nodeID string) {
	lb.mu.RLock()
	cb, exists := lb.circuitBreakers[nodeID]
	lb.mu.RUnlock()
	
	if exists {
		cb.RecordSuccess()
	}
}

// RecordNodeFailure records a failed operation for a node
func (lb *LoadBalancer) RecordNodeFailure(nodeID string) {
	lb.mu.RLock()
	cb, exists := lb.circuitBreakers[nodeID]
	lb.mu.RUnlock()
	
	if exists {
		cb.RecordFailure()
	}
}

// GetNodeStatus returns the status of all nodes
func (lb *LoadBalancer) GetNodeStatus() map[string]*models.NodeStatus {
	lb.mu.RLock()
	defer lb.mu.RUnlock()
	
	result := make(map[string]*models.NodeStatus)
	for k, v := range lb.nodes {
		result[k] = v
	}
	return result
}

// GetNodeCapacity returns the capacity of a node
func (lb *LoadBalancer) GetNodeCapacity(nodeID string) *models.NodeCapacity {
	lb.mu.RLock()
	defer lb.mu.RUnlock()
	
	return lb.capacities[nodeID]
}

// getHealthyNodes returns only healthy nodes with circuit breaker checks
func (lb *LoadBalancer) getHealthyNodes() []string {
	var healthy []string
	for nodeID, status := range lb.nodes {
		if status.IsHealthy() {
			// Check circuit breaker
			lb.mu.RLock()
			cb, exists := lb.circuitBreakers[nodeID]
			lb.mu.RUnlock()
			
			if !exists || cb.IsAvailable() {
				healthy = append(healthy, nodeID)
			}
		}
	}
	return healthy
}

// roundRobin routes tasks in round-robin fashion with circuit breaker awareness
func (lb *LoadBalancer) roundRobin(healthyNodes []string) (string, error) {
	if len(healthyNodes) == 0 {
		return "", fmt.Errorf("no healthy nodes available")
	}
	
	lb.lastIndex = (lb.lastIndex + 1) % len(healthyNodes)
	selectedNode := healthyNodes[lb.lastIndex]
	
	// Record the selection for metrics
	lb.recordNodeSelection(selectedNode, "round_robin")
	
	return selectedNode, nil
}

// leastLoaded routes to the node with the lowest load with advanced load calculation
func (lb *LoadBalancer) leastLoaded(healthyNodes []string, task *models.Task) (string, error) {
	if len(healthyNodes) == 0 {
		return "", fmt.Errorf("no healthy nodes available")
	}
	
	var bestNode string
	var lowestLoad float64 = 1.0
	
	for _, nodeID := range healthyNodes {
		status := lb.nodes[nodeID]
		capacity := lb.capacities[nodeID]
		
		if capacity == nil {
			continue
		}
		
		// Check if node can accept the task
		if !capacity.CanAcceptTask(task.EstimatedTokens) {
			continue
		}
		
		// Calculate comprehensive load including queue, active tasks, and capacity
		load := lb.calculateComprehensiveLoad(status, capacity, task)
		
		if load < lowestLoad {
			lowestLoad = load
			bestNode = nodeID
		}
	}
	
	if bestNode == "" {
		return "", fmt.Errorf("no nodes can accept task with %d estimated tokens", task.EstimatedTokens)
	}
	
	lb.recordNodeSelection(bestNode, "least_loaded")
	return bestNode, nil
}

// calculateComprehensiveLoad calculates load considering multiple factors
func (lb *LoadBalancer) calculateComprehensiveLoad(status *models.NodeStatus, capacity *models.NodeCapacity, task *models.Task) float64 {
	// Active task load
	activeLoad := float64(status.ActiveTasks) / float64(capacity.MaxConcurrentTasks)
	
	// Queue load
	queueLoad := float64(status.QueueLength) / float64(capacity.MaxQueueSize)
	
			// Token load
		tokenLoad := float64(capacity.CurrentTokensPerMin) / float64(capacity.MaxTokensPerMinute)
	
	// Weighted combination
	comprehensiveLoad := activeLoad*0.5 + queueLoad*0.3 + tokenLoad*0.2
	
	// Add penalty for large tasks
	if task.EstimatedTokens > capacity.MaxTokensPerMinute/2 {
		comprehensiveLoad += 0.1
	}
	
	return comprehensiveLoad
}

// weighted routes based on node capacity and current load with advanced scoring
func (lb *LoadBalancer) weighted(healthyNodes []string, task *models.Task) (string, error) {
	if len(healthyNodes) == 0 {
		return "", fmt.Errorf("no healthy nodes available")
	}
	
	var bestNode string
	var bestScore float64 = -1
	
	for _, nodeID := range healthyNodes {
		status := lb.nodes[nodeID]
		capacity := lb.capacities[nodeID]
		
		if capacity == nil {
			continue
		}
		
		// Check if node can accept the task
		if !capacity.CanAcceptTask(task.EstimatedTokens) {
			continue
		}
		
		// Calculate advanced weighted score
		score := lb.calculateWeightedScore(status, capacity, task)
		
		if score > bestScore {
			bestScore = score
			bestNode = nodeID
		}
	}
	
	if bestNode == "" {
		return "", fmt.Errorf("no nodes can accept task with %d estimated tokens", task.EstimatedTokens)
	}
	
	lb.recordNodeSelection(bestNode, "weighted")
	return bestNode, nil
}

// calculateWeightedScore calculates a comprehensive weighted score
func (lb *LoadBalancer) calculateWeightedScore(status *models.NodeStatus, capacity *models.NodeCapacity, task *models.Task) float64 {
	// Capacity score (higher capacity = better)
	capacityScore := float64(capacity.MaxTokensPerMinute) / 10000.0
	
	// Load score (lower load = better)
	loadScore := 1.0 - (float64(status.ActiveTasks) / float64(capacity.MaxConcurrentTasks))
	
	// Queue score (lower queue = better)
	queueScore := 1.0 - (float64(status.QueueLength) / float64(capacity.MaxQueueSize))
	
			// Token utilization score (lower utilization = better)
		tokenScore := 1.0 - (float64(capacity.CurrentTokensPerMin) / float64(capacity.MaxTokensPerMinute))
	
	// Health score (based on last heartbeat)
	healthScore := 1.0
	if time.Since(status.LastHeartbeat) > 30*time.Second {
		healthScore = 0.5
	}
	
	// Weighted combination
	score := capacityScore*0.25 + loadScore*0.25 + queueScore*0.2 + tokenScore*0.2 + healthScore*0.1
	
	return score
}

// random routes to a random healthy node with weighted randomness
func (lb *LoadBalancer) random(healthyNodes []string) (string, error) {
	if len(healthyNodes) == 0 {
		return "", fmt.Errorf("no healthy nodes available")
	}
	
	// Use weighted random selection based on node capacity
	totalWeight := 0
	weights := make([]int, len(healthyNodes))
	
	for i, nodeID := range healthyNodes {
		if capacity := lb.capacities[nodeID]; capacity != nil {
			weight := capacity.MaxTokensPerMinute / 1000 // Normalize weight
			weights[i] = weight
			totalWeight += weight
		} else {
			weights[i] = 1
			totalWeight += 1
		}
	}
	
	// Select weighted random node
	randValue := lb.rand.Intn(totalWeight)
	currentWeight := 0
	
	for i, weight := range weights {
		currentWeight += weight
		if randValue < currentWeight {
			selectedNode := healthyNodes[i]
			lb.recordNodeSelection(selectedNode, "random")
			return selectedNode, nil
		}
	}
	
	// Fallback to simple random
	index := lb.rand.Intn(len(healthyNodes))
	selectedNode := healthyNodes[index]
	lb.recordNodeSelection(selectedNode, "random")
	return selectedNode, nil
}

// adaptive routes using adaptive load balancing based on current conditions
func (lb *LoadBalancer) adaptive(healthyNodes []string, task *models.Task) (string, error) {
	if len(healthyNodes) == 0 {
		return "", fmt.Errorf("no healthy nodes available")
	}
	
	// Analyze current system state
	systemLoad := lb.calculateSystemLoad(healthyNodes)
	nodeCount := len(healthyNodes)
	
	// Choose strategy based on conditions
	if systemLoad > 0.8 {
		// High load - use least loaded
		return lb.leastLoaded(healthyNodes, task)
	} else if nodeCount < 3 {
		// Few nodes - use weighted
		return lb.weighted(healthyNodes, task)
	} else if task.Priority > 5 {
		// High priority - use least loaded
		return lb.leastLoaded(healthyNodes, task)
	} else {
		// Normal conditions - use weighted random
		return lb.weightedRandom(healthyNodes, task)
	}
}

// weightedRandom combines weighted and random selection
func (lb *LoadBalancer) weightedRandom(healthyNodes []string, task *models.Task) (string, error) {
	if len(healthyNodes) == 0 {
		return "", fmt.Errorf("no healthy nodes available")
	}
	
	// Calculate scores for all nodes
	scores := make([]float64, len(healthyNodes))
	var totalScore float64
	
	for i, nodeID := range healthyNodes {
		status := lb.nodes[nodeID]
		capacity := lb.capacities[nodeID]
		
		if capacity == nil {
			scores[i] = 0.1
			totalScore += 0.1
			continue
		}
		
		if !capacity.CanAcceptTask(task.EstimatedTokens) {
			scores[i] = 0
			continue
		}
		
		score := lb.calculateWeightedScore(status, capacity, task)
		scores[i] = score
		totalScore += score
	}
	
	if totalScore == 0 {
		return "", fmt.Errorf("no suitable nodes available")
	}
	
	// Select based on weighted random
	randValue := lb.rand.Float64() * totalScore
	currentScore := 0.0
	
	for i, score := range scores {
		currentScore += score
		if randValue <= currentScore {
			selectedNode := healthyNodes[i]
			lb.recordNodeSelection(selectedNode, "adaptive")
			return selectedNode, nil
		}
	}
	
	// Fallback
	selectedNode := healthyNodes[0]
	lb.recordNodeSelection(selectedNode, "adaptive")
	return selectedNode, nil
}

// geographic routes based on geographic proximity (placeholder for real implementation)
func (lb *LoadBalancer) geographic(healthyNodes []string, task *models.Task) (string, error) {
	// In a real implementation, this would consider geographic location
	// For now, fall back to weighted selection
	return lb.weighted(healthyNodes, task)
}

// calculateSystemLoad calculates the overall system load
func (lb *LoadBalancer) calculateSystemLoad(healthyNodes []string) float64 {
	if len(healthyNodes) == 0 {
		return 0
	}
	
	var totalLoad float64
	for _, nodeID := range healthyNodes {
		status := lb.nodes[nodeID]
		if status != nil {
			totalLoad += status.GetLoad()
		}
	}
	
	return totalLoad / float64(len(healthyNodes))
}

// recordNodeSelection records node selection for metrics
func (lb *LoadBalancer) recordNodeSelection(nodeID, strategy string) {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	
	if selections, exists := lb.metrics["node_selections"].(map[string]int); exists {
		selections[nodeID]++
	} else {
		lb.metrics["node_selections"] = map[string]int{nodeID: 1}
	}
	
	if strategies, exists := lb.metrics["strategy_usage"].(map[string]int); exists {
		strategies[strategy]++
	} else {
		lb.metrics["strategy_usage"] = map[string]int{strategy: 1}
	}
}

// GetStrategy returns the current load balancing strategy
func (lb *LoadBalancer) GetStrategy() string {
	return lb.strategy
}

// GetStats returns comprehensive load balancer statistics
func (lb *LoadBalancer) GetStats() map[string]interface{} {
	lb.mu.RLock()
	defer lb.mu.RUnlock()
	
	healthyNodes := lb.getHealthyNodes()
	
	stats := map[string]interface{}{
		"strategy":      lb.strategy,
		"total_nodes":   len(lb.nodes),
		"healthy_nodes": len(healthyNodes),
		"system_load":   lb.calculateSystemLoad(healthyNodes),
		"nodes":         make(map[string]interface{}),
		"metrics":       lb.metrics,
		"circuit_breakers": make(map[string]interface{}),
	}
	
	for nodeID, status := range lb.nodes {
		nodeStats := map[string]interface{}{
			"healthy":     status.IsHealthy(),
			"active_tasks": status.ActiveTasks,
			"queue_length": status.QueueLength,
			"load":        status.GetLoad(),
		}
		
		if capacity := lb.capacities[nodeID]; capacity != nil {
			nodeStats["capacity"] = map[string]interface{}{
				"max_concurrent_tasks": capacity.MaxConcurrentTasks,
				"max_tokens_per_minute": capacity.MaxTokensPerMinute,
				"current_concurrent_tasks": capacity.CurrentConcurrentTasks,
				"current_tokens_per_minute": capacity.CurrentTokensPerMin,
			}
		}
		
		if cb := lb.circuitBreakers[nodeID]; cb != nil {
			nodeStats["circuit_breaker"] = map[string]interface{}{
				"state": cb.state,
				"available": cb.IsAvailable(),
			}
		}
		
		stats["nodes"].(map[string]interface{})[nodeID] = nodeStats
	}
	
	return stats
} 