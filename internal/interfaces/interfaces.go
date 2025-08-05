package interfaces

import (
	"context"
	"llm-balancer/internal/models"
	"time"
)

// TaskProcessor defines the interface for processing tasks
type TaskProcessor interface {
	// ProcessTask processes a single task and returns the result
	ProcessTask(ctx context.Context, task *models.Task) (*models.TaskResult, error)
	
	// EstimateTokens estimates the number of tokens for a task
	EstimateTokens(task *models.Task) int
	
	// GetCapacity returns the current capacity information
	GetCapacity() *models.NodeCapacity
}

// LoadBalancer defines the interface for load balancing
type LoadBalancer interface {
	// RouteTask routes a task to the best available node
	RouteTask(ctx context.Context, task *models.Task) (string, error)
	
	// GetNodeStatus returns the status of all nodes
	GetNodeStatus() map[string]*models.NodeStatus
	
	// RegisterNode registers a new node
	RegisterNode(nodeID string, status *models.NodeStatus) error
	
	// UnregisterNode unregisters a node
	UnregisterNode(nodeID string) error
	
	// UpdateNodeStatus updates the status of a node
	UpdateNodeStatus(nodeID string, status *models.NodeStatus) error
}

// NodeManager defines the interface for managing individual nodes
type NodeManager interface {
	// Start starts the node manager
	Start(ctx context.Context) error
	
	// Stop stops the node manager
	Stop() error
	
	// SubmitTask submits a task to the node
	SubmitTask(ctx context.Context, task *models.Task) error
	
	// GetStatus returns the current status of the node
	GetStatus() *models.NodeStatus
	
	// GetCapacity returns the current capacity of the node
	GetCapacity() *models.NodeCapacity
	
	// IsHealthy returns true if the node is healthy
	IsHealthy() bool
}

// TaskQueue defines the interface for task queues
type TaskQueue interface {
	// Push adds a task to the queue
	Push(task *models.Task) error
	
	// Pop removes and returns the highest priority task
	Pop() (*models.Task, error)
	
	// PopWithTimeout removes and returns the highest priority task with timeout
	PopWithTimeout(timeout time.Duration) (*models.Task, error)
	
	// Peek returns the highest priority task without removing it
	Peek() (*models.Task, error)
	
	// Size returns the current number of items in the queue
	Size() int
	
	// IsEmpty returns true if the queue is empty
	IsEmpty() bool
	
	// IsFull returns true if the queue is at capacity
	IsFull() bool
	
	// Clear removes all tasks from the queue
	Clear()
	
	// Close closes the queue
	Close()
}

// MetricsCollector defines the interface for collecting metrics
type MetricsCollector interface {
	// RecordTaskSubmission records a task submission
	RecordTaskSubmission(nodeID string, task *models.Task)
	
	// RecordTaskCompletion records a task completion
	RecordTaskCompletion(nodeID string, task *models.Task, duration time.Duration)
	
	// RecordTaskFailure records a task failure
	RecordTaskFailure(nodeID string, task *models.Task, error string)
	
	// RecordNodeHealth records node health status
	RecordNodeHealth(nodeID string, healthy bool)
	
	// GetMetrics returns current metrics
	GetMetrics() map[string]interface{}
}

// HealthChecker defines the interface for health checking
type HealthChecker interface {
	// CheckHealth checks the health of a node
	CheckHealth(nodeID string) bool
	
	// StartHealthCheck starts periodic health checking
	StartHealthCheck(ctx context.Context)
	
	// StopHealthCheck stops health checking
	StopHealthCheck()
}

// DiscoveryService defines the interface for node discovery
type DiscoveryService interface {
	// DiscoverNodes discovers available nodes
	DiscoverNodes(ctx context.Context) ([]string, error)
	
	// RegisterNode registers a node with the discovery service
	RegisterNode(nodeID string, address string) error
	
	// UnregisterNode unregisters a node from the discovery service
	UnregisterNode(nodeID string) error
	
	// GetNodes returns all registered nodes
	GetNodes() map[string]string
} 