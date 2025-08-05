package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
	
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"llm-balancer/internal/balancer"
	"llm-balancer/internal/config"
	"llm-balancer/internal/models"
	"bytes"
)

type GatewayServer struct {
	config     *config.Config
	balancer   *balancer.LoadBalancer
	logger     *logrus.Logger
	httpServer *http.Server
	httpClient *http.Client
	
	// Production features
	taskRegistry map[string]*TaskInfo
	mu           sync.RWMutex
	metrics      map[string]interface{}
}

type TaskInfo struct {
	TaskID      string                 `json:"task_id"`
	NodeID      string                 `json:"node_id"`
	Status      string                 `json:"status"`
	CreatedAt   time.Time              `json:"created_at"`
	LastUpdated time.Time              `json:"last_updated"`
	RetryCount  int                    `json:"retry_count"`
	MaxRetries  int                    `json:"max_retries"`
	Payload     []byte                 `json:"payload"`
	Parameters  map[string]interface{} `json:"parameters"`
	Priority    int                    `json:"priority"`
	Result      interface{}            `json:"result,omitempty"`
	Error       string                 `json:"error,omitempty"`
}

type TaskRequest struct {
	Payload    string                 `json:"payload"`
	Parameters map[string]interface{} `json:"parameters"`
	Priority   int                    `json:"priority"`
}

type TaskResponse struct {
	TaskID   string                 `json:"task_id"`
	NodeID   string                 `json:"node_id"`
	Status   string                 `json:"status"`
	Message  string                 `json:"message,omitempty"`
}

func main() {
	// Parse command line arguments
	if len(os.Args) < 2 {
		log.Fatal("Usage: go run cmd/gateway/main.go <config-file>")
	}
	
	configPath := os.Args[1]
	
	// Load configuration
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	
	// Initialize logger
	logger := logrus.New()
	logger.SetLevel(logrus.InfoLevel)
	if cfg.Logging.Format == "json" {
		logger.SetFormatter(&logrus.JSONFormatter{})
	}
	
	// Create load balancer
	loadBalancer := balancer.NewLoadBalancer(cfg.Gateway.LoadBalancingStrategy)
	
	// Create HTTP client with timeouts
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
		},
	}
	
	// Create gateway server
	server := &GatewayServer{
		config:       cfg,
		balancer:     loadBalancer,
		logger:       logger,
		httpClient:   httpClient,
		taskRegistry: make(map[string]*TaskInfo),
		metrics:      make(map[string]interface{}),
	}
	
	// Start the server
	if err := server.Start(); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

func (s *GatewayServer) Start() error {
	// Set up Gin router
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery())
	
	// API routes
	api := router.Group("/api/v1")
	{
		// Task management
		api.POST("/tasks", s.handleSubmitTask)
		api.GET("/tasks/:taskId", s.handleGetTask)
		api.GET("/tasks", s.handleGetAllTasks)
		
		// Node management
		api.GET("/nodes", s.handleGetNodes)
		api.GET("/nodes/count", s.handleGetNodeCount)
		api.GET("/nodes/health", s.handleGetNodeHealth)
		api.GET("/nodes/:nodeId", s.handleGetNodeDetails)
		api.POST("/nodes", s.handleRegisterNode)
		api.DELETE("/nodes/:nodeId", s.handleUnregisterNode)
		
		// System monitoring
		api.GET("/stats", s.handleGetStats)
		api.GET("/stats/overview", s.handleGetSystemOverview)
		api.GET("/stats/tasks", s.handleGetTaskStats)
		api.GET("/stats/capacity", s.handleGetCapacityStats)
		
		// Health and status
		api.GET("/health", s.handleHealth)
		api.GET("/status", s.handleGetSystemStatus)
	}
	
	// Health check
	router.GET("/health", s.handleHealth)
	
	// Create HTTP server
	s.httpServer = &http.Server{
		Addr:    fmt.Sprintf("%s:%d", s.config.Gateway.Host, s.config.Gateway.Port),
		Handler: router,
	}
	
	// Start server in goroutine
	go func() {
		s.logger.Infof("Starting gateway server on %s:%d", s.config.Gateway.Host, s.config.Gateway.Port)
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Fatalf("Failed to start server: %v", err)
		}
	}()
	
	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	
	s.logger.Info("Shutting down gateway server...")
	
	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	if err := s.httpServer.Shutdown(ctx); err != nil {
		s.logger.Errorf("Server forced to shutdown: %v", err)
	}
	
	s.logger.Info("Gateway server exited")
	return nil
}

func (s *GatewayServer) handleSubmitTask(c *gin.Context) {
	var req TaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}
	
	// Validate request
	if len(req.Payload) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Payload cannot be empty"})
		return
	}
	
	if req.Priority < 0 || req.Priority > 10 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Priority must be between 0 and 10"})
		return
	}
	
	// Create task
	task := models.NewTask(
		generateTaskID(),
		[]byte(req.Payload),
		req.Parameters,
	)
	task.Priority = req.Priority
	
	// Estimate tokens (improved estimation)
	task.EstimatedTokens = s.estimateTokens([]byte(req.Payload), req.Parameters)
	
	// Route task
	nodeID, err := s.balancer.RouteTask(c.Request.Context(), task)
	if err != nil {
		s.logger.Errorf("Failed to route task: %v", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "No available nodes to process task",
		})
		return
	}
	
	// Submit task to selected node
	success, err := s.submitTaskToNode(nodeID, task)
	if err != nil {
		s.logger.Errorf("Failed to submit task to node %s: %v", nodeID, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to submit task to node",
		})
		return
	}
	
	if !success {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Node is not accepting tasks",
		})
		return
	}
	
	// Register task in our registry
	taskInfo := &TaskInfo{
		TaskID:      task.ID,
		NodeID:      nodeID,
		Status:      "submitted",
		CreatedAt:   time.Now(),
		LastUpdated: time.Now(),
		RetryCount:  0,
		MaxRetries:  3,
		Payload:     []byte(req.Payload),
		Parameters:  req.Parameters,
		Priority:    req.Priority,
	}
	
	s.registerTask(taskInfo)
	
	// Update metrics
	s.updateTaskMetrics("submitted", nodeID)
	
	response := TaskResponse{
		TaskID:  task.ID,
		NodeID:  nodeID,
		Status:  "submitted",
		Message: "Task submitted to node",
	}
	
	s.logger.Infof("Task %s submitted to node %s", task.ID, nodeID)
	c.JSON(http.StatusOK, response)
}

func (s *GatewayServer) estimateTokens(payload []byte, parameters map[string]interface{}) int {
	// Base estimation
	baseTokens := len(payload) / 4
	
	// Adjust based on parameters
	if parameters != nil {
		if maxTokens, ok := parameters["max_tokens"].(int); ok && maxTokens > 0 {
			// Use provided max_tokens as a guide
			baseTokens = maxTokens
		}
		
		if model, ok := parameters["model"].(string); ok {
			// Adjust based on model complexity
			switch model {
			case "gpt-4":
				baseTokens = int(float64(baseTokens) * 1.2)
			case "gpt-3.5-turbo":
				baseTokens = int(float64(baseTokens) * 1.0)
			default:
				baseTokens = int(float64(baseTokens) * 1.1)
			}
		}
	}
	
	// Ensure minimum tokens
	if baseTokens < 100 {
		baseTokens = 100
	}
	
	return baseTokens
}

func (s *GatewayServer) submitTaskToNode(nodeID string, task *models.Task) (bool, error) {
	// Get node address
	nodes := s.balancer.GetNodeStatus()
	nodeStatus, exists := nodes[nodeID]
	if !exists {
		return false, fmt.Errorf("node %s not found", nodeID)
	}
	
	// Prepare request
	taskData := map[string]interface{}{
		"payload":    task.Payload,
		"parameters": task.Parameters,
		"priority":   task.Priority,
	}
	
	jsonData, err := json.Marshal(taskData)
	if err != nil {
		return false, err
	}
	
	// Submit to node
	url := fmt.Sprintf("http://%s/api/v1/tasks", nodeStatus.Address)
	resp, err := s.httpClient.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	
	return resp.StatusCode == http.StatusOK, nil
}

func (s *GatewayServer) handleGetTask(c *gin.Context) {
	taskID := c.Param("taskId")
	
	// Get task from registry
	taskInfo := s.getTaskInfo(taskID)
	if taskInfo == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Task not found",
		})
		return
	}
	
	// Get current status from node
	currentStatus, err := s.getTaskStatusFromNode(taskInfo.NodeID, taskID)
	if err != nil {
		s.logger.Warnf("Failed to get task status from node: %v", err)
		// Return cached status if we can't reach the node
		c.JSON(http.StatusOK, taskInfo)
		return
	}
	
			// Update task info with current status
		taskInfo.Status = string(currentStatus.Status)
	taskInfo.LastUpdated = time.Now()
	
	// If task is completed or failed, get the result
	if currentStatus.Status == "completed" || currentStatus.Status == "failed" {
		if currentStatus.Result != nil {
			taskInfo.Result = currentStatus.Result
		}
		if currentStatus.Error != "" {
			taskInfo.Error = currentStatus.Error
		}
	}
	
	c.JSON(http.StatusOK, taskInfo)
}

func (s *GatewayServer) getTaskStatusFromNode(nodeID, taskID string) (*models.Task, error) {
	nodes := s.balancer.GetNodeStatus()
	nodeStatus, exists := nodes[nodeID]
	if !exists {
		return nil, fmt.Errorf("node %s not found", nodeID)
	}
	
	url := fmt.Sprintf("http://%s/api/v1/tasks/%s", nodeStatus.Address, taskID)
	resp, err := s.httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("node returned status %d", resp.StatusCode)
	}
	
	var task models.Task
	if err := json.NewDecoder(resp.Body).Decode(&task); err != nil {
		return nil, err
	}
	
	return &task, nil
}

func (s *GatewayServer) handleGetAllTasks(c *gin.Context) {
	s.mu.RLock()
	tasks := make([]*TaskInfo, 0, len(s.taskRegistry))
	for _, task := range s.taskRegistry {
		tasks = append(tasks, task)
	}
	s.mu.RUnlock()
	
	// Get current status for all tasks
	var updatedTasks []map[string]interface{}
	for _, task := range tasks {
		taskData := map[string]interface{}{
			"task_id":      task.TaskID,
			"node_id":      task.NodeID,
			"status":       task.Status,
			"created_at":   task.CreatedAt,
			"last_updated": task.LastUpdated,
			"priority":     task.Priority,
		}
		
		// Try to get current status from node
		if currentStatus, err := s.getTaskStatusFromNode(task.NodeID, task.TaskID); err == nil {
			taskData["status"] = string(currentStatus.Status)
			taskData["last_updated"] = time.Now()
		}
		
		updatedTasks = append(updatedTasks, taskData)
	}
	
	c.JSON(http.StatusOK, gin.H{
		"tasks": updatedTasks,
		"total_tasks": len(tasks),
		"by_status": s.getTaskCountsByStatus(),
	})
}

func (s *GatewayServer) getTaskCountsByStatus() map[string]int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	counts := make(map[string]int)
	for _, task := range s.taskRegistry {
		counts[task.Status]++
	}
	return counts
}

func (s *GatewayServer) registerTask(taskInfo *TaskInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	s.taskRegistry[taskInfo.TaskID] = taskInfo
}

func (s *GatewayServer) getTaskInfo(taskID string) *TaskInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	return s.taskRegistry[taskID]
}

func (s *GatewayServer) updateTaskMetrics(status, nodeID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if metrics, exists := s.metrics["task_metrics"].(map[string]interface{}); exists {
		if statusCounts, ok := metrics["by_status"].(map[string]int); ok {
			statusCounts[status]++
		}
		if nodeCounts, ok := metrics["by_node"].(map[string]int); ok {
			nodeCounts[nodeID]++
		}
	} else {
		s.metrics["task_metrics"] = map[string]interface{}{
			"by_status": map[string]int{status: 1},
			"by_node":   map[string]int{nodeID: 1},
		}
	}
}

func (s *GatewayServer) handleGetNodes(c *gin.Context) {
	nodes := s.balancer.GetNodeStatus()
	
	var nodeList []map[string]interface{}
	
	for nodeID, nodeStatus := range nodes {
		node := map[string]interface{}{
			"node_id": nodeID,
			"address": nodeStatus.Address,
			"healthy": nodeStatus.IsHealthy(),
			"last_heartbeat": nodeStatus.LastHeartbeat,
			"active_tasks": nodeStatus.ActiveTasks,
			"queue_length": nodeStatus.QueueLength,
			"load": nodeStatus.GetLoad(),
		}
		nodeList = append(nodeList, node)
	}
	
	c.JSON(http.StatusOK, gin.H{
		"nodes": nodeList,
		"total_nodes": len(nodes),
		"healthy_nodes": len(s.getHealthyNodes()),
	})
}

func (s *GatewayServer) handleGetStats(c *gin.Context) {
	// Get load balancer stats
	balancerStats := s.balancer.GetStats()
	
	// Get system overview
	nodes := s.balancer.GetNodeStatus()
	healthyNodes := s.getHealthyNodes()
	
	// Calculate additional stats
	var totalActiveTasks int
	var totalQueueLength int
	
	for _, nodeStatus := range nodes {
		totalActiveTasks += nodeStatus.ActiveTasks
		totalQueueLength += nodeStatus.QueueLength
	}
	
	// Get task registry stats
	s.mu.RLock()
	taskCounts := s.getTaskCountsByStatus()
	totalTasks := len(s.taskRegistry)
	s.mu.RUnlock()
	
	stats := map[string]interface{}{
		"balancer": balancerStats,
		"system": map[string]interface{}{
			"total_nodes": len(nodes),
			"healthy_nodes": len(healthyNodes),
			"unhealthy_nodes": len(nodes) - len(healthyNodes),
			"health_percentage": float64(len(healthyNodes)) / float64(len(nodes)) * 100,
		},
		"tasks": map[string]interface{}{
			"total_active": totalActiveTasks,
			"total_queued": totalQueueLength,
			"total_tasks": totalActiveTasks + totalQueueLength,
			"registry_tasks": totalTasks,
			"by_status": taskCounts,
		},
		"gateway_metrics": s.metrics,
		"timestamp": time.Now().UTC(),
	}
	
	c.JSON(http.StatusOK, stats)
}

func (s *GatewayServer) handleRegisterNode(c *gin.Context) {
	var req struct {
		NodeID  string `json:"node_id"`
		Address string `json:"address"`
	}
	
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}
	
	status := models.NewNodeStatus(req.NodeID, req.Address)
	if err := s.balancer.RegisterNode(req.NodeID, status); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to register node"})
		return
	}
	
	s.logger.Infof("Registered node %s at %s", req.NodeID, req.Address)
	c.JSON(http.StatusOK, gin.H{"message": "Node registered successfully"})
}

func (s *GatewayServer) handleUnregisterNode(c *gin.Context) {
	nodeID := c.Param("nodeId")
	
	if err := s.balancer.UnregisterNode(nodeID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to unregister node"})
		return
	}
	
	s.logger.Infof("Unregistered node %s", nodeID)
	c.JSON(http.StatusOK, gin.H{"message": "Node unregistered successfully"})
}

func (s *GatewayServer) handleHealth(c *gin.Context) {
	// Comprehensive health check
	health := map[string]interface{}{
		"status": "healthy",
		"time":   time.Now().UTC(),
		"checks": map[string]interface{}{
			"balancer_healthy": len(s.getHealthyNodes()) > 0,
			"registry_healthy": true, // Add more checks as needed
		},
		"metrics": map[string]interface{}{
			"total_nodes": len(s.balancer.GetNodeStatus()),
			"healthy_nodes": len(s.getHealthyNodes()),
			"total_tasks": len(s.taskRegistry),
		},
	}
	
	c.JSON(http.StatusOK, health)
}

func generateTaskID() string {
	return fmt.Sprintf("task-%d", time.Now().UnixNano())
}

func (s *GatewayServer) handleGetNodeCount(c *gin.Context) {
	nodes := s.balancer.GetNodeStatus()
	healthyNodes := s.getHealthyNodes()
	
	c.JSON(http.StatusOK, gin.H{
		"total_nodes": len(nodes),
		"healthy_nodes": len(healthyNodes),
		"unhealthy_nodes": len(nodes) - len(healthyNodes),
	})
}

func (s *GatewayServer) handleGetNodeHealth(c *gin.Context) {
	nodes := s.balancer.GetNodeStatus()
	
	var healthStatus []map[string]interface{}
	
	for nodeID, nodeStatus := range nodes {
		health := map[string]interface{}{
			"node_id": nodeID,
			"address": nodeStatus.Address,
			"healthy": nodeStatus.IsHealthy(),
			"last_heartbeat": nodeStatus.LastHeartbeat,
			"active_tasks": nodeStatus.ActiveTasks,
			"queue_length": nodeStatus.QueueLength,
			"load": nodeStatus.GetLoad(),
		}
		healthStatus = append(healthStatus, health)
	}
	
	c.JSON(http.StatusOK, gin.H{
		"nodes": healthStatus,
		"total_nodes": len(nodes),
		"healthy_count": len(s.getHealthyNodes()),
	})
}

func (s *GatewayServer) handleGetNodeDetails(c *gin.Context) {
	nodeID := c.Param("nodeId")
	
	nodes := s.balancer.GetNodeStatus()
	nodeStatus, exists := nodes[nodeID]
	
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Node not found",
		})
		return
	}
	
	// Get node capacity
	nodeCapacity := s.balancer.GetNodeCapacity(nodeID)
	
	details := map[string]interface{}{
		"node_id": nodeID,
		"address": nodeStatus.Address,
		"status": map[string]interface{}{
			"healthy": nodeStatus.IsHealthy(),
			"last_heartbeat": nodeStatus.LastHeartbeat,
			"active_tasks": nodeStatus.ActiveTasks,
			"queue_length": nodeStatus.QueueLength,
			"load": nodeStatus.GetLoad(),
		},
	}
	
	if nodeCapacity != nil {
		details["capacity"] = map[string]interface{}{
			"max_requests_per_minute": nodeCapacity.MaxRequestsPerMinute,
			"max_tokens_per_minute": nodeCapacity.MaxTokensPerMinute,
			"max_concurrent_tasks": nodeCapacity.MaxConcurrentTasks,
			"max_queue_size": nodeCapacity.MaxQueueSize,
			"current_requests_per_min": nodeCapacity.CurrentRequestsPerMin,
			"current_tokens_per_min": nodeCapacity.CurrentTokensPerMin,
			"current_concurrent_tasks": nodeCapacity.CurrentConcurrentTasks,
		}
	}
	
	c.JSON(http.StatusOK, details)
}

func (s *GatewayServer) handleGetSystemOverview(c *gin.Context) {
	nodes := s.balancer.GetNodeStatus()
	healthyNodes := s.getHealthyNodes()
	
	// Calculate system-wide statistics
	var totalActiveTasks int
	var totalQueueLength int
	var totalLoad float64
	
	for _, nodeStatus := range nodes {
		totalActiveTasks += nodeStatus.ActiveTasks
		totalQueueLength += nodeStatus.QueueLength
		totalLoad += nodeStatus.GetLoad()
	}
	
	avgLoad := 0.0
	if len(nodes) > 0 {
		avgLoad = totalLoad / float64(len(nodes))
	}
	
	// Get task registry overview
	s.mu.RLock()
	taskCounts := s.getTaskCountsByStatus()
	totalTasks := len(s.taskRegistry)
	s.mu.RUnlock()
	
	overview := map[string]interface{}{
		"system": map[string]interface{}{
			"total_nodes": len(nodes),
			"healthy_nodes": len(healthyNodes),
			"unhealthy_nodes": len(nodes) - len(healthyNodes),
			"health_percentage": float64(len(healthyNodes)) / float64(len(nodes)) * 100,
		},
		"tasks": map[string]interface{}{
			"total_active": totalActiveTasks,
			"total_queued": totalQueueLength,
			"total_tasks": totalActiveTasks + totalQueueLength,
			"registry_tasks": totalTasks,
			"by_status": taskCounts,
		},
		"performance": map[string]interface{}{
			"average_load": avgLoad,
			"total_load": totalLoad,
		},
		"balancer": map[string]interface{}{
			"strategy": s.balancer.GetStrategy(),
			"last_updated": time.Now(),
		},
	}
	
	c.JSON(http.StatusOK, overview)
}

func (s *GatewayServer) handleGetTaskStats(c *gin.Context) {
	nodes := s.balancer.GetNodeStatus()
	
	var taskStats []map[string]interface{}
	
	for nodeID, nodeStatus := range nodes {
		stats := map[string]interface{}{
			"node_id": nodeID,
			"address": nodeStatus.Address,
			"active_tasks": nodeStatus.ActiveTasks,
			"queue_length": nodeStatus.QueueLength,
			"total_tasks": nodeStatus.ActiveTasks + nodeStatus.QueueLength,
		}
		taskStats = append(taskStats, stats)
	}
	
	// Calculate totals
	var totalActive, totalQueued, totalTasks int
	for _, stats := range taskStats {
		totalActive += stats["active_tasks"].(int)
		totalQueued += stats["queued_tasks"].(int)
		totalTasks += stats["total_tasks"].(int)
	}
	
	// Get registry stats
	s.mu.RLock()
	taskCounts := s.getTaskCountsByStatus()
	totalRegistryTasks := len(s.taskRegistry)
	s.mu.RUnlock()
	
	c.JSON(http.StatusOK, gin.H{
		"nodes": taskStats,
		"totals": map[string]interface{}{
			"active_tasks": totalActive,
			"queued_tasks": totalQueued,
			"total_tasks": totalTasks,
		},
		"registry": map[string]interface{}{
			"total_tasks": totalRegistryTasks,
			"by_status": taskCounts,
		},
		"summary": map[string]interface{}{
			"nodes_with_tasks": len(taskStats),
			"average_tasks_per_node": float64(totalTasks) / float64(len(nodes)),
		},
	})
}

func (s *GatewayServer) handleGetCapacityStats(c *gin.Context) {
	nodes := s.balancer.GetNodeStatus()
	
	var capacityStats []map[string]interface{}
	
	for nodeID, nodeStatus := range nodes {
		nodeCapacity := s.balancer.GetNodeCapacity(nodeID)
		
		stats := map[string]interface{}{
			"node_id": nodeID,
			"address": nodeStatus.Address,
			"healthy": nodeStatus.IsHealthy(),
		}
		
		if nodeCapacity != nil {
			stats["capacity"] = map[string]interface{}{
				"max_requests_per_minute": nodeCapacity.MaxRequestsPerMinute,
				"max_tokens_per_minute": nodeCapacity.MaxTokensPerMinute,
				"max_concurrent_tasks": nodeCapacity.MaxConcurrentTasks,
				"max_queue_size": nodeCapacity.MaxQueueSize,
				"current_requests_per_min": nodeCapacity.CurrentRequestsPerMin,
				"current_tokens_per_min": nodeCapacity.CurrentTokensPerMin,
				"current_concurrent_tasks": nodeCapacity.CurrentConcurrentTasks,
				"utilization_percentage": float64(nodeCapacity.CurrentConcurrentTasks) / float64(nodeCapacity.MaxConcurrentTasks) * 100,
			}
		}
		
		capacityStats = append(capacityStats, stats)
	}
	
	// Calculate system-wide capacity
	var totalMaxRequests, totalMaxTokens, totalMaxConcurrent, totalMaxQueue int
	var totalCurrentRequests, totalCurrentTokens, totalCurrentConcurrent int
	
	for _, stats := range capacityStats {
		if capacity, exists := stats["capacity"].(map[string]interface{}); exists {
			totalMaxRequests += capacity["max_requests_per_minute"].(int)
			totalMaxTokens += capacity["max_tokens_per_minute"].(int)
			totalMaxConcurrent += capacity["max_concurrent_tasks"].(int)
			totalMaxQueue += capacity["max_queue_size"].(int)
			totalCurrentRequests += capacity["current_requests_per_min"].(int)
			totalCurrentTokens += capacity["current_tokens_per_min"].(int)
			totalCurrentConcurrent += capacity["current_concurrent_tasks"].(int)
		}
	}
	
	c.JSON(http.StatusOK, gin.H{
		"nodes": capacityStats,
		"system_capacity": map[string]interface{}{
			"max_requests_per_minute": totalMaxRequests,
			"max_tokens_per_minute": totalMaxTokens,
			"max_concurrent_tasks": totalMaxConcurrent,
			"max_queue_size": totalMaxQueue,
			"current_requests_per_min": totalCurrentRequests,
			"current_tokens_per_min": totalCurrentTokens,
			"current_concurrent_tasks": totalCurrentConcurrent,
			"utilization_percentage": float64(totalCurrentConcurrent) / float64(totalMaxConcurrent) * 100,
		},
	})
}

func (s *GatewayServer) handleGetSystemStatus(c *gin.Context) {
	nodes := s.balancer.GetNodeStatus()
	healthyNodes := s.getHealthyNodes()
	
	// Determine overall system status
	systemStatus := "healthy"
	if len(healthyNodes) == 0 {
		systemStatus = "down"
	} else if len(healthyNodes) < len(nodes) {
		systemStatus = "degraded"
	}
	
	// Get task registry status
	s.mu.RLock()
	taskCounts := s.getTaskCountsByStatus()
	totalTasks := len(s.taskRegistry)
	s.mu.RUnlock()
	
	status := map[string]interface{}{
		"status": systemStatus,
		"timestamp": time.Now().UTC(),
		"nodes": map[string]interface{}{
			"total": len(nodes),
			"healthy": len(healthyNodes),
			"unhealthy": len(nodes) - len(healthyNodes),
		},
		"tasks": map[string]interface{}{
			"total": totalTasks,
			"by_status": taskCounts,
		},
		"balancer": map[string]interface{}{
			"strategy": s.balancer.GetStrategy(),
			"available": len(healthyNodes) > 0,
		},
		"uptime": "0h 0m 0s", // In a real implementation, track actual uptime
	}
	
	c.JSON(http.StatusOK, status)
}

// getHealthyNodes returns only healthy nodes
func (s *GatewayServer) getHealthyNodes() []string {
	var healthy []string
	nodes := s.balancer.GetNodeStatus()
	
	for nodeID, nodeStatus := range nodes {
		if nodeStatus.IsHealthy() {
			healthy = append(healthy, nodeID)
		}
	}
	
	return healthy
} 