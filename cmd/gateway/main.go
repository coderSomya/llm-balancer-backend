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
		api.POST("/tasks", s.handleSubmitTask)
		api.GET("/tasks/:taskId", s.handleGetTask)
		api.GET("/nodes", s.handleGetNodes)
		api.GET("/stats", s.handleGetStats)
		api.POST("/nodes", s.handleRegisterNode)
		api.DELETE("/nodes/:nodeId", s.handleUnregisterNode)
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
	c.JSON(http.StatusOK, gin.H{
		"nodes": nodes,
	})
}

func (s *GatewayServer) handleGetStats(c *gin.Context) {
	stats := s.balancer.GetStats()
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