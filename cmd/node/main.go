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
	"llm-balancer/internal/config"
	"llm-balancer/internal/models"
	"llm-balancer/internal/queue"
	"llm-balancer/internal/node"
)

type NodeServer struct {
	config       *config.Config
	queue        *queue.TaskQueue
	status       *models.NodeStatus
	capacity     *models.NodeCapacity
	logger       *logrus.Logger
	httpServer   *http.Server
	ctx          context.Context
	cancel       context.CancelFunc
	gossipMgr    *node.GossipManager
	taskTracker  *node.TaskTracker
}

func main() {
	// Parse command line arguments
	if len(os.Args) < 2 {
		log.Fatal("Usage: go run cmd/node/main.go <config-file>")
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
	
	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	
	// Create task queue
	taskQueue := queue.NewTaskQueue(cfg.Node.MaxQueueSize)
	
	// Create node status and capacity
	nodeStatus := models.NewNodeStatus(cfg.Node.ID, cfg.Node.Address)
	nodeCapacity := models.NewNodeCapacity(
		cfg.Node.MaxRequestsPerMinute,
		cfg.Node.MaxTokensPerMinute,
		cfg.Node.MaxConcurrentTasks,
		cfg.Node.MaxQueueSize,
	)
	
	// Create gossip manager and task tracker
	gossipMgr := node.NewGossipManager(cfg.Node.ID, cfg.Node.Address, taskQueue)
	taskTracker := node.NewTaskTracker()
	
	// Create node server
	server := &NodeServer{
		config:      cfg,
		queue:       taskQueue,
		status:      nodeStatus,
		capacity:    nodeCapacity,
		logger:      logger,
		ctx:         ctx,
		cancel:      cancel,
		gossipMgr:   gossipMgr,
		taskTracker: taskTracker,
	}
	
	// Start the server
	if err := server.Start(); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

func (s *NodeServer) Start() error {
	// Start background tasks
	go s.startHeartbeat()
	go s.startCapacityReset()
	go s.startQueueProcessor()
	
	// Start gossip protocol
	s.gossipMgr.Start()
	
	// Set up Gin router
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery())
	
	// API routes
	api := router.Group("/api/v1")
	{
		api.POST("/tasks", s.handleSubmitTask)
		api.GET("/tasks/:taskId", s.handleGetTask)
		api.GET("/tasks/failed", s.handleGetFailedTasks)
		api.GET("/status", s.handleGetStatus)
		api.GET("/capacity", s.handleGetCapacity)
		api.GET("/queue/stats", s.handleGetQueueStats)
		api.POST("/gossip", s.handleGossip)
		api.GET("/peers", s.handleGetPeers)
	}
	
	// Health check
	router.GET("/health", s.handleHealth)
	
	// Create HTTP server
	s.httpServer = &http.Server{
		Addr:    fmt.Sprintf("%s:%d", s.config.Server.Host, s.config.Server.Port),
		Handler: router,
	}
	
	// Start server in goroutine
	go func() {
		s.logger.Infof("Starting node server on %s:%d", s.config.Server.Host, s.config.Server.Port)
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Fatalf("Failed to start server: %v", err)
		}
	}()
	
	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	
	s.logger.Info("Shutting down node server...")
	
	// Cancel context to stop background tasks
	s.cancel()
	
	// Stop gossip protocol
	s.gossipMgr.Stop()
	
	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	if err := s.httpServer.Shutdown(ctx); err != nil {
		s.logger.Errorf("Server forced to shutdown: %v", err)
	}
	
	// Close queue
	s.queue.Close()
	
	s.logger.Info("Node server exited")
	return nil
}

func (s *NodeServer) startHeartbeat() {
	ticker := time.NewTicker(s.config.Node.HeartbeatInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.status.UpdateHealth(true)
			s.status.UpdateTaskCounts(s.capacity.CurrentConcurrentTasks, s.queue.Size())
			s.logger.Debugf("Heartbeat sent - Active tasks: %d, Queue size: %d", 
				s.capacity.CurrentConcurrentTasks, s.queue.Size())
		}
	}
}

func (s *NodeServer) startCapacityReset() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.capacity.ResetMinuteCounts()
			s.logger.Debug("Capacity counters reset")
		}
	}
}

func (s *NodeServer) startQueueProcessor() {
	// Start worker pool
	for i := 0; i < s.config.Node.WorkerPoolSize; i++ {
		go s.worker(i)
	}
}

func (s *NodeServer) worker(id int) {
	s.logger.Infof("Worker %d started", id)
	
	for {
		select {
		case <-s.ctx.Done():
			s.logger.Infof("Worker %d stopped", id)
			return
		default:
			// Try to get a task from the queue
			task, err := s.queue.PopWithTimeout(5 * time.Second)
			if err != nil {
				if err == queue.ErrTimeout || err == queue.ErrQueueEmpty {
					continue
				}
				s.logger.Errorf("Worker %d failed to pop task: %v", id, err)
				continue
			}
			
			// Process the task
			s.processTask(task)
		}
	}
}

func (s *NodeServer) processTask(task *models.Task) {
	startTime := time.Now()
	
	// Update task status
	task.Status = models.TaskStatusRunning
	task.StartedAt = &startTime
	task.NodeID = s.config.Node.ID
	
	// Update task in tracker
	s.taskTracker.UpdateTask(task.ID, func(t *models.Task) {
		t.Status = models.TaskStatusRunning
		t.StartedAt = &startTime
		t.NodeID = s.config.Node.ID
	})
	
	// Increment capacity counters
	s.capacity.IncrementTaskCounts(task.EstimatedTokens)
	
	s.logger.Infof("Processing task %s (worker)", task.ID)
	
	// Simulate task processing (replace with actual LLM processing)
	time.Sleep(time.Duration(len(task.Payload)/100) * time.Millisecond)
	
	// Update task status
	completedTime := time.Now()
	task.Status = models.TaskStatusCompleted
	task.CompletedAt = &completedTime
	
	// Create result
	task.Result = &models.TaskResult{
		Response:       []byte(fmt.Sprintf("Processed: %s", string(task.Payload))),
		TokensUsed:     task.EstimatedTokens,
		ProcessingTime: completedTime.Sub(startTime),
		Metadata: map[string]interface{}{
			"worker_id": "worker-1",
			"node_id":   s.config.Node.ID,
		},
	}
	
	// Update task in tracker
	s.taskTracker.UpdateTask(task.ID, func(t *models.Task) {
		t.Status = models.TaskStatusCompleted
		t.CompletedAt = &completedTime
		t.Result = task.Result
	})
	
	// Decrement capacity counters
	s.capacity.DecrementTaskCounts(task.EstimatedTokens)
	
	s.logger.Infof("Completed task %s in %v", task.ID, task.Result.ProcessingTime)
}

func (s *NodeServer) handleSubmitTask(c *gin.Context) {
	var req struct {
		Payload    []byte                 `json:"payload"`
		Parameters map[string]interface{} `json:"parameters"`
		Priority   int                    `json:"priority"`
	}
	
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
	task.EstimatedTokens = len(req.Payload) / 4 // Rough estimation
	
	// Add task to tracker
	s.taskTracker.AddTask(task)
	
	// Check if queue is full
	if s.queue.IsFull() {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Node queue is full",
		})
		return
	}
	
	// Add task to queue
	if err := s.queue.Push(task); err != nil {
		s.logger.Errorf("Failed to add task to queue: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to queue task",
		})
		return
	}
	
	s.logger.Infof("Task %s queued successfully", task.ID)
	c.JSON(http.StatusOK, gin.H{
		"task_id": task.ID,
		"status":  "queued",
		"message": "Task added to queue",
	})
}

func (s *NodeServer) handleGetTask(c *gin.Context) {
	taskID := c.Param("taskId")
	
	// Get task from tracker
	task, exists := s.taskTracker.GetTask(taskID)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Task not found",
		})
		return
	}
	
	c.JSON(http.StatusOK, task)
}

func (s *NodeServer) handleGetFailedTasks(c *gin.Context) {
	// Get tasks that can be redistributed
	tasks := s.taskTracker.GetTasksForRedistribution()
	
	c.JSON(http.StatusOK, gin.H{
		"tasks": tasks,
		"count": len(tasks),
	})
}

func (s *NodeServer) handleGetStatus(c *gin.Context) {
	c.JSON(http.StatusOK, s.status)
}

func (s *NodeServer) handleGetCapacity(c *gin.Context) {
	c.JSON(http.StatusOK, s.capacity)
}

func (s *NodeServer) handleGetQueueStats(c *gin.Context) {
	stats := s.queue.GetStats()
	c.JSON(http.StatusOK, stats)
}

func (s *NodeServer) handleGossip(c *gin.Context) {
	var message map[string]interface{}
	if err := c.ShouldBindJSON(&message); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid gossip message"})
		return
	}
	
	// Handle the gossip message
	s.gossipMgr.HandleGossipMessage(message)
	
	c.JSON(http.StatusOK, gin.H{"status": "received"})
}

func (s *NodeServer) handleGetPeers(c *gin.Context) {
	peers := s.gossipMgr.GetPeers()
	c.JSON(http.StatusOK, gin.H{
		"peers": peers,
		"count": len(peers),
	})
}

func (s *NodeServer) handleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "healthy",
		"node_id": s.config.Node.ID,
		"time":   time.Now().UTC(),
	})
}

func generateTaskID() string {
	return fmt.Sprintf("task-%d", time.Now().UnixNano())
} 