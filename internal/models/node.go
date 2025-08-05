package models

import (
	"sync"
	"time"
)

// NodeStatus represents the current state of a node
type NodeStatus struct {
	ID              string    `json:"id"`
	Address         string    `json:"address"`
	RequestsPerMin  int       `json:"requests_per_min"`
	TokensPerMin    int       `json:"tokens_per_min"`
	CurrentLoad     float64   `json:"current_load"`
	Health          bool      `json:"health"`
	LastHeartbeat   time.Time `json:"last_heartbeat"`
	ActiveTasks     int       `json:"active_tasks"`
	QueueLength     int       `json:"queue_length"`
	mu              sync.RWMutex
}

// NodeCapacity represents the capacity constraints of a node
type NodeCapacity struct {
	MaxRequestsPerMinute int     `json:"max_requests_per_minute"`
	MaxTokensPerMinute   int     `json:"max_tokens_per_minute"`
	MaxConcurrentTasks   int     `json:"max_concurrent_tasks"`
	MaxQueueSize         int     `json:"max_queue_size"`
	CurrentRequestsPerMin int    `json:"current_requests_per_min"`
	CurrentTokensPerMin   int    `json:"current_tokens_per_min"`
	CurrentConcurrentTasks int   `json:"current_concurrent_tasks"`
	mu                   sync.RWMutex
}

// NewNodeStatus creates a new node status
func NewNodeStatus(id, address string) *NodeStatus {
	return &NodeStatus{
		ID:            id,
		Address:       address,
		Health:        true,
		LastHeartbeat: time.Now(),
	}
}

// NewNodeCapacity creates a new node capacity with default values
func NewNodeCapacity(maxRequests, maxTokens, maxConcurrent, maxQueue int) *NodeCapacity {
	return &NodeCapacity{
		MaxRequestsPerMinute: maxRequests,
		MaxTokensPerMinute:   maxTokens,
		MaxConcurrentTasks:   maxConcurrent,
		MaxQueueSize:         maxQueue,
	}
}

// UpdateLoad updates the current load of the node
func (ns *NodeStatus) UpdateLoad(load float64) {
	ns.mu.Lock()
	defer ns.mu.Unlock()
	ns.CurrentLoad = load
	ns.LastHeartbeat = time.Now()
}

// UpdateHealth updates the health status
func (ns *NodeStatus) UpdateHealth(health bool) {
	ns.mu.Lock()
	defer ns.mu.Unlock()
	ns.Health = health
	ns.LastHeartbeat = time.Now()
}

// UpdateTaskCounts updates the active tasks and queue length
func (ns *NodeStatus) UpdateTaskCounts(activeTasks, queueLength int) {
	ns.mu.Lock()
	defer ns.mu.Unlock()
	ns.ActiveTasks = activeTasks
	ns.QueueLength = queueLength
	ns.LastHeartbeat = time.Now()
}

// GetLoad returns the current load (thread-safe)
func (ns *NodeStatus) GetLoad() float64 {
	ns.mu.RLock()
	defer ns.mu.RUnlock()
	return ns.CurrentLoad
}

// IsHealthy returns the health status (thread-safe)
func (ns *NodeStatus) IsHealthy() bool {
	ns.mu.RLock()
	defer ns.mu.RUnlock()
	return ns.Health && time.Since(ns.LastHeartbeat) < 30*time.Second
}

// CanAcceptTask checks if the node can accept a new task
func (nc *NodeCapacity) CanAcceptTask(estimatedTokens int) bool {
	nc.mu.RLock()
	defer nc.mu.RUnlock()
	
	return nc.CurrentConcurrentTasks < nc.MaxConcurrentTasks &&
		nc.CurrentTokensPerMin+estimatedTokens <= nc.MaxTokensPerMinute
}

// IncrementTaskCounts increments the current task counts
func (nc *NodeCapacity) IncrementTaskCounts(estimatedTokens int) {
	nc.mu.Lock()
	defer nc.mu.Unlock()
	nc.CurrentConcurrentTasks++
	nc.CurrentTokensPerMin += estimatedTokens
}

// DecrementTaskCounts decrements the current task counts
func (nc *NodeCapacity) DecrementTaskCounts(estimatedTokens int) {
	nc.mu.Lock()
	defer nc.mu.Unlock()
	if nc.CurrentConcurrentTasks > 0 {
		nc.CurrentConcurrentTasks--
	}
	if nc.CurrentTokensPerMin >= estimatedTokens {
		nc.CurrentTokensPerMin -= estimatedTokens
	}
}

// ResetMinuteCounts resets the per-minute counters (should be called every minute)
func (nc *NodeCapacity) ResetMinuteCounts() {
	nc.mu.Lock()
	defer nc.mu.Unlock()
	nc.CurrentRequestsPerMin = 0
	nc.CurrentTokensPerMin = 0
} 