package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"llm-balancer/internal/balancer"
	"llm-balancer/internal/config"
	"llm-balancer/internal/models"
)

type GatewayServer struct {
	config     *config.Config
	balancer   *balancer.LoadBalancer
	logger     *logrus.Logger
	httpServer *http.Server
}

type TaskRequest struct {
	Payload    []byte                 `json:"payload"`
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
	
	// Create gateway server
	server := &GatewayServer{
		config:   cfg,
		balancer: loadBalancer,
		logger:   logger,
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
	
	// Create task
	task := models.NewTask(
		generateTaskID(),
		req.Payload,
		req.Parameters,
	)
	task.Priority = req.Priority
	
	// Estimate tokens (simple estimation for now)
	task.EstimatedTokens = len(req.Payload) / 4 // Rough estimation
	
	// Route task
	nodeID, err := s.balancer.RouteTask(c.Request.Context(), task)
	if err != nil {
		s.logger.Errorf("Failed to route task: %v", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "No available nodes to process task",
		})
		return
	}
	
	// For now, just return the routing decision
	response := TaskResponse{
		TaskID:  task.ID,
		NodeID:  nodeID,
		Status:  "routed",
		Message: "Task routed to node",
	}
	
	s.logger.Infof("Task %s routed to node %s", task.ID, nodeID)
	c.JSON(http.StatusOK, response)
}

func (s *GatewayServer) handleGetTask(c *gin.Context) {
	taskID := c.Param("taskId")
	
	// For now, return a mock response
	c.JSON(http.StatusOK, gin.H{
		"task_id": taskID,
		"status":  "pending",
		"message": "Task status not implemented yet",
	})
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
		},
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
	c.JSON(http.StatusOK, gin.H{
		"status": "healthy",
		"time":   time.Now().UTC(),
	})
}

func generateTaskID() string {
	return fmt.Sprintf("task-%d", time.Now().UnixNano())
} 

func (s *GatewayServer) handleGetAllTasks(c *gin.Context) {
	// Get all nodes
	nodes := s.balancer.GetNodeStatus()
	
	var allTasks []map[string]interface{}
	
	// Collect tasks from all nodes
	for nodeID, nodeStatus := range nodes {
		if nodeStatus.IsHealthy() {
			// In a real implementation, we would query each node for its tasks
			// For now, return mock data
			nodeTasks := map[string]interface{}{
				"node_id": nodeID,
				"address": nodeStatus.Address,
				"tasks": []map[string]interface{}{
					{
						"id":     "task-1",
						"status": "completed",
						"created_at": time.Now().Add(-5 * time.Minute),
					},
					{
						"id":     "task-2", 
						"status": "running",
						"created_at": time.Now().Add(-2 * time.Minute),
					},
				},
				"total_tasks": 2,
			}
			allTasks = append(allTasks, nodeTasks)
		}
	}
	
	c.JSON(http.StatusOK, gin.H{
		"tasks": allTasks,
		"total_nodes": len(nodes),
		"healthy_nodes": len(s.getHealthyNodes()),
	})
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
		},
		"performance": map[string]interface{}{
			"average_load": avgLoad,
			"total_load": totalLoad,
		},
		"balancer": map[string]interface{}{
			"strategy": s.balancer.strategy,
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
		totalQueued += stats["queue_length"].(int)
		totalTasks += stats["total_tasks"].(int)
	}
	
	c.JSON(http.StatusOK, gin.H{
		"nodes": taskStats,
		"totals": map[string]interface{}{
			"active_tasks": totalActive,
			"queued_tasks": totalQueued,
			"total_tasks": totalTasks,
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
	
	status := map[string]interface{}{
		"status": systemStatus,
		"timestamp": time.Now().UTC(),
		"nodes": map[string]interface{}{
			"total": len(nodes),
			"healthy": len(healthyNodes),
			"unhealthy": len(nodes) - len(healthyNodes),
		},
		"balancer": map[string]interface{}{
			"strategy": s.balancer.strategy,
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