package balancer

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"
	"llm-balancer/internal/models"
	"llm-balancer/internal/interfaces"
)

// LoadBalancer implements intelligent task routing
type LoadBalancer struct {
	nodes       map[string]*models.NodeStatus
	capacities  map[string]*models.NodeCapacity
	strategy    string
	mu          sync.RWMutex
	lastIndex   int
	rand        *rand.Rand
}

// NewLoadBalancer creates a new load balancer
func NewLoadBalancer(strategy string) *LoadBalancer {
	return &LoadBalancer{
		nodes:      make(map[string]*models.NodeStatus),
		capacities: make(map[string]*models.NodeCapacity),
		strategy:   strategy,
		rand:       rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// RouteTask routes a task to the best available node
func (lb *LoadBalancer) RouteTask(ctx context.Context, task *models.Task) (string, error) {
	lb.mu.RLock()
	defer lb.mu.RUnlock()
	
	// Get healthy nodes
	healthyNodes := lb.getHealthyNodes()
	if len(healthyNodes) == 0 {
		return "", fmt.Errorf("no healthy nodes available")
	}
	
	// Route based on strategy
	switch lb.strategy {
	case "round_robin":
		return lb.roundRobin(healthyNodes)
	case "least_loaded":
		return lb.leastLoaded(healthyNodes, task)
	case "weighted":
		return lb.weighted(healthyNodes, task)
	case "random":
		return lb.random(healthyNodes)
	default:
		return lb.roundRobin(healthyNodes)
	}
}

// RegisterNode registers a new node
func (lb *LoadBalancer) RegisterNode(nodeID string, status *models.NodeStatus) error {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	
	lb.nodes[nodeID] = status
	return nil
}

// UnregisterNode unregisters a node
func (lb *LoadBalancer) UnregisterNode(nodeID string) error {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	
	delete(lb.nodes, nodeID)
	delete(lb.capacities, nodeID)
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

// getHealthyNodes returns only healthy nodes
func (lb *LoadBalancer) getHealthyNodes() []string {
	var healthy []string
	for nodeID, status := range lb.nodes {
		if status.IsHealthy() {
			healthy = append(healthy, nodeID)
		}
	}
	return healthy
}

// roundRobin routes tasks in round-robin fashion
func (lb *LoadBalancer) roundRobin(healthyNodes []string) (string, error) {
	if len(healthyNodes) == 0 {
		return "", fmt.Errorf("no healthy nodes available")
	}
	
	lb.lastIndex = (lb.lastIndex + 1) % len(healthyNodes)
	return healthyNodes[lb.lastIndex], nil
}

// leastLoaded routes to the node with the lowest load
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
		
		// Calculate load (active tasks / max concurrent tasks)
		load := float64(status.ActiveTasks) / float64(capacity.MaxConcurrentTasks)
		
		if load < lowestLoad {
			lowestLoad = load
			bestNode = nodeID
		}
	}
	
	if bestNode == "" {
		return "", fmt.Errorf("no nodes can accept task with %d estimated tokens", task.EstimatedTokens)
	}
	
	return bestNode, nil
}

// weighted routes based on node capacity and current load
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
		
		// Calculate weighted score
		// Higher capacity and lower load = better score
		capacityScore := float64(capacity.MaxTokensPerMinute) / 10000.0 // Normalize
		loadScore := 1.0 - (float64(status.ActiveTasks) / float64(capacity.MaxConcurrentTasks))
		queueScore := 1.0 - (float64(status.QueueLength) / float64(capacity.MaxQueueSize))
		
		score := capacityScore * 0.4 + loadScore * 0.4 + queueScore * 0.2
		
		if score > bestScore {
			bestScore = score
			bestNode = nodeID
		}
	}
	
	if bestNode == "" {
		return "", fmt.Errorf("no nodes can accept task with %d estimated tokens", task.EstimatedTokens)
	}
	
	return bestNode, nil
}

// random routes to a random healthy node
func (lb *LoadBalancer) random(healthyNodes []string) (string, error) {
	if len(healthyNodes) == 0 {
		return "", fmt.Errorf("no healthy nodes available")
	}
	
	index := lb.rand.Intn(len(healthyNodes))
	return healthyNodes[index], nil
}

// GetStats returns load balancer statistics
func (lb *LoadBalancer) GetStats() map[string]interface{} {
	lb.mu.RLock()
	defer lb.mu.RUnlock()
	
	stats := map[string]interface{}{
		"strategy":     lb.strategy,
		"total_nodes":  len(lb.nodes),
		"healthy_nodes": len(lb.getHealthyNodes()),
		"nodes":        make(map[string]interface{}),
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
				"current_tokens_per_minute": capacity.CurrentTokensPerMinute,
			}
		}
		
		stats["nodes"].(map[string]interface{})[nodeID] = nodeStats
	}
	
	return stats
} 