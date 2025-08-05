package node

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"sync"
	"time"
	"llm-balancer/internal/models"
	"llm-balancer/internal/queue"
)

// GossipNode represents a node in the gossip network
type GossipNode struct {
	ID        string    `json:"id"`
	Address   string    `json:"address"`
	LastSeen  time.Time `json:"last_seen"`
	Status    string    `json:"status"` // "alive", "suspected", "dead"
	Tasks     []string  `json:"tasks,omitempty"`
	Capacity  *models.NodeCapacity `json:"capacity,omitempty"`
	Load      float64   `json:"load"`
	Latency   time.Duration `json:"latency"`
}

// GossipManager handles node-to-node communication and failure detection
type GossipManager struct {
	nodeID       string
	address      string
	peers        map[string]*GossipNode
	mu           sync.RWMutex
	queue        *queue.TaskQueue
	httpClient   *http.Client
	ctx          context.Context
	cancel       context.CancelFunc
	
	// Configuration
	gossipInterval    time.Duration
	failureTimeout    time.Duration
	suspicionTimeout  time.Duration
	maxGossipPeers    int
	
	// Advanced features
	peerLatencyHistory map[string][]time.Duration
	loadBalancingStrategy string
	healthCheckInterval time.Duration
}

// NewGossipManager creates a new gossip manager
func NewGossipManager(nodeID, address string, queue *queue.TaskQueue) *GossipManager {
	ctx, cancel := context.WithCancel(context.Background())
	
	return &GossipManager{
		nodeID:           nodeID,
		address:          address,
		peers:            make(map[string]*GossipNode),
		queue:            queue,
		httpClient:       &http.Client{Timeout: 5 * time.Second},
		ctx:              ctx,
		cancel:           cancel,
		gossipInterval:   10 * time.Second,
		failureTimeout:   30 * time.Second,
		suspicionTimeout: 15 * time.Second,
		maxGossipPeers:   10,
		peerLatencyHistory: make(map[string][]time.Duration),
		loadBalancingStrategy: "weighted",
		healthCheckInterval: 5 * time.Second,
	}
}

// Start begins the gossip protocol
func (gm *GossipManager) Start() {
	go gm.gossipLoop()
	go gm.failureDetectionLoop()
	go gm.taskRedistributionLoop()
	go gm.healthCheckLoop()
	go gm.latencyMonitoringLoop()
}

// Stop stops the gossip protocol
func (gm *GossipManager) Stop() {
	gm.cancel()
}

// AddPeer adds a new peer to the gossip network
func (gm *GossipManager) AddPeer(nodeID, address string) {
	gm.mu.Lock()
	defer gm.mu.Unlock()
	
	gm.peers[nodeID] = &GossipNode{
		ID:       nodeID,
		Address:  address,
		LastSeen: time.Now(),
		Status:   "alive",
		Load:     0.0,
		Latency:  0,
	}
}

// RemovePeer removes a peer from the gossip network
func (gm *GossipManager) RemovePeer(nodeID string) {
	gm.mu.Lock()
	defer gm.mu.Unlock()
	
	delete(gm.peers, nodeID)
	delete(gm.peerLatencyHistory, nodeID)
}

// GetPeers returns all known peers
func (gm *GossipManager) GetPeers() map[string]*GossipNode {
	gm.mu.RLock()
	defer gm.mu.RUnlock()
	
	result := make(map[string]*GossipNode)
	for k, v := range gm.peers {
		result[k] = v
	}
	return result
}

// gossipLoop periodically gossips with other nodes
func (gm *GossipManager) gossipLoop() {
	ticker := time.NewTicker(gm.gossipInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-gm.ctx.Done():
			return
		case <-ticker.C:
			gm.gossip()
		}
	}
}

// gossip sends gossip messages to random peers
func (gm *GossipManager) gossip() {
	gm.mu.RLock()
	peers := make([]*GossipNode, 0, len(gm.peers))
	for _, peer := range gm.peers {
		peers = append(peers, peer)
	}
	gm.mu.RUnlock()
	
	// Select random peers to gossip with using crypto/rand
	selectedPeers := gm.selectRandomPeers(peers, 3)
	
	for _, peer := range selectedPeers {
		go gm.sendGossipMessage(peer)
	}
}

// selectRandomPeers selects up to maxCount random peers using crypto/rand
func (gm *GossipManager) selectRandomPeers(peers []*GossipNode, maxCount int) []*GossipNode {
	if len(peers) <= maxCount {
		return peers
	}
	
	selected := make([]*GossipNode, 0, maxCount)
	used := make(map[int]bool)
	
	for len(selected) < maxCount {
		// Use crypto/rand for secure random selection
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(peers))))
		if err != nil {
			// Fallback to time-based selection if crypto/rand fails
			idx := int(time.Now().UnixNano()) % len(peers)
			if !used[idx] {
				selected = append(selected, peers[idx])
				used[idx] = true
			}
			time.Sleep(1 * time.Millisecond)
		} else {
			idx := int(n.Int64())
			if !used[idx] {
				selected = append(selected, peers[idx])
				used[idx] = true
			}
		}
	}
	
	return selected
}

// sendGossipMessage sends a gossip message to a peer with latency measurement
func (gm *GossipManager) sendGossipMessage(peer *GossipNode) {
	message := gm.createGossipMessage()
	
	url := fmt.Sprintf("http://%s/api/v1/gossip", peer.Address)
	
	start := time.Now()
	resp, err := gm.httpClient.Post(url, "application/json", bytes.NewBuffer(message))
	latency := time.Since(start)
	
	if err != nil {
		gm.markPeerSuspicious(peer.ID)
		return
	}
	defer resp.Body.Close()
	
	if resp.StatusCode == http.StatusOK {
		gm.markPeerAlive(peer.ID)
		gm.updatePeerLatency(peer.ID, latency)
	} else {
		gm.markPeerSuspicious(peer.ID)
	}
}

// updatePeerLatency updates the latency history for a peer
func (gm *GossipManager) updatePeerLatency(nodeID string, latency time.Duration) {
	gm.mu.Lock()
	defer gm.mu.Unlock()
	
	history := gm.peerLatencyHistory[nodeID]
	if len(history) >= 10 {
		// Keep only last 10 measurements
		history = history[1:]
	}
	history = append(history, latency)
	gm.peerLatencyHistory[nodeID] = history
	
	// Update peer latency
	if peer, exists := gm.peers[nodeID]; exists {
		peer.Latency = gm.calculateAverageLatency(history)
	}
}

// calculateAverageLatency calculates the average latency from history
func (gm *GossipManager) calculateAverageLatency(history []time.Duration) time.Duration {
	if len(history) == 0 {
		return 0
	}
	
	var total time.Duration
	for _, lat := range history {
		total += lat
	}
	return total / time.Duration(len(history))
}

// createGossipMessage creates a comprehensive gossip message
func (gm *GossipManager) createGossipMessage() []byte {
	gm.mu.RLock()
	peers := make(map[string]interface{})
	for nodeID, peer := range gm.peers {
		peers[nodeID] = map[string]interface{}{
			"id":         peer.ID,
			"address":    peer.Address,
			"last_seen":  peer.LastSeen.Unix(),
			"status":     peer.Status,
			"load":       peer.Load,
			"latency":    peer.Latency.Nanoseconds(),
		}
	}
	gm.mu.RUnlock()
	
	message := map[string]interface{}{
		"node_id":  gm.nodeID,
		"address":  gm.address,
		"timestamp": time.Now().Unix(),
		"peers":    peers,
		"version":  "1.0",
	}
	
	jsonData, _ := json.Marshal(message)
	return jsonData
}

// markPeerAlive marks a peer as alive
func (gm *GossipManager) markPeerAlive(nodeID string) {
	gm.mu.Lock()
	defer gm.mu.Unlock()
	
	if peer, exists := gm.peers[nodeID]; exists {
		peer.LastSeen = time.Now()
		peer.Status = "alive"
	}
}

// markPeerSuspicious marks a peer as suspicious
func (gm *GossipManager) markPeerSuspicious(nodeID string) {
	gm.mu.Lock()
	defer gm.mu.Unlock()
	
	if peer, exists := gm.peers[nodeID]; exists {
		peer.Status = "suspected"
	}
}

// markPeerDead marks a peer as dead
func (gm *GossipManager) markPeerDead(nodeID string) {
	gm.mu.Lock()
	defer gm.mu.Unlock()
	
	if peer, exists := gm.peers[nodeID]; exists {
		peer.Status = "dead"
	}
}

// failureDetectionLoop periodically checks for failed nodes with advanced detection
func (gm *GossipManager) failureDetectionLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-gm.ctx.Done():
			return
		case <-ticker.C:
			gm.detectFailures()
		}
	}
}

// detectFailures checks for nodes that have failed with advanced heuristics
func (gm *GossipManager) detectFailures() {
	gm.mu.Lock()
	defer gm.mu.Unlock()
	
	now := time.Now()
	
	for nodeID, peer := range gm.peers {
		timeSinceLastSeen := now.Sub(peer.LastSeen)
		
		switch peer.Status {
		case "alive":
			if timeSinceLastSeen > gm.suspicionTimeout {
				peer.Status = "suspected"
			}
		case "suspected":
			if timeSinceLastSeen > gm.failureTimeout {
				peer.Status = "dead"
				go gm.handleNodeFailure(nodeID)
			}
		case "dead":
			// Remove dead peers after a grace period
			if timeSinceLastSeen > 5*time.Minute {
				delete(gm.peers, nodeID)
				delete(gm.peerLatencyHistory, nodeID)
			}
		}
	}
}

// handleNodeFailure handles the failure of a node with advanced recovery
func (gm *GossipManager) handleNodeFailure(failedNodeID string) {
	// Try to redistribute tasks from the failed node
	gm.redistributeTasks(failedNodeID)
	
	// Remove the failed peer
	gm.RemovePeer(failedNodeID)
}

// redistributeTasks attempts to redistribute tasks from a failed node with intelligent selection
func (gm *GossipManager) redistributeTasks(failedNodeID string) {
	// Get tasks from the failed node
	tasks, err := gm.getTasksFromNode(failedNodeID)
	if err != nil {
		return
	}
	
	// Find available peers to redistribute tasks
	availablePeers := gm.getAvailablePeers()
	if len(availablePeers) == 0 {
		return
	}
	
	// Sort tasks by priority for redistribution
	gm.sortTasksByPriority(tasks)
	
	// Redistribute tasks with intelligent load balancing
	for _, task := range tasks {
		// Find the best peer for this task using advanced selection
		bestPeer := gm.selectBestPeerForTask(task, availablePeers)
		if bestPeer != nil {
			success := gm.sendTaskToPeer(task, bestPeer)
			if success {
				// Update peer load after successful redistribution
				gm.updatePeerLoad(bestPeer.ID, task.EstimatedTokens)
			}
		}
	}
}

// sortTasksByPriority sorts tasks by priority (highest first)
func (gm *GossipManager) sortTasksByPriority(tasks []*models.Task) {
	// Simple bubble sort for priority (in production, use sort.Slice)
	for i := 0; i < len(tasks)-1; i++ {
		for j := 0; j < len(tasks)-i-1; j++ {
			if tasks[j].Priority < tasks[j+1].Priority {
				tasks[j], tasks[j+1] = tasks[j+1], tasks[j]
			}
		}
	}
}

// updatePeerLoad updates the load of a peer after task assignment
func (gm *GossipManager) updatePeerLoad(nodeID string, tokens int) {
	gm.mu.Lock()
	defer gm.mu.Unlock()
	
	if peer, exists := gm.peers[nodeID]; exists {
		// Simple load calculation (in production, use more sophisticated metrics)
		peer.Load += float64(tokens) / 1000.0 // Normalize load
	}
}

// getTasksFromNode attempts to get tasks from a failed node with retry logic
func (gm *GossipManager) getTasksFromNode(nodeID string) ([]*models.Task, error) {
	gm.mu.RLock()
	peer, exists := gm.peers[nodeID]
	gm.mu.RUnlock()
	
	if !exists {
		return nil, fmt.Errorf("peer not found")
	}
	
	// Try to get tasks from the failed node with retry
	maxRetries := 3
	for attempt := 0; attempt < maxRetries; attempt++ {
		tasks, err := gm.fetchTasksFromNode(peer.Address)
		if err == nil {
			return tasks, nil
		}
		
		// Wait before retry
		time.Sleep(time.Duration(attempt+1) * time.Second)
	}
	
	return nil, fmt.Errorf("failed to get tasks from node after %d attempts", maxRetries)
}

// fetchTasksFromNode fetches tasks from a specific node
func (gm *GossipManager) fetchTasksFromNode(address string) ([]*models.Task, error) {
	url := fmt.Sprintf("http://%s/api/v1/tasks/failed", address)
	resp, err := gm.httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get tasks from node: status %d", resp.StatusCode)
	}
	
	var tasks []*models.Task
	if err := json.NewDecoder(resp.Body).Decode(&tasks); err != nil {
		return nil, err
	}
	
	return tasks, nil
}

// getAvailablePeers returns peers that are alive and can accept tasks
func (gm *GossipManager) getAvailablePeers() []*GossipNode {
	gm.mu.RLock()
	defer gm.mu.RUnlock()
	
	var available []*GossipNode
	for _, peer := range gm.peers {
		if peer.Status == "alive" {
			available = append(available, peer)
		}
	}
	return available
}

// selectBestPeerForTask selects the best peer for a given task using advanced algorithms
func (gm *GossipManager) selectBestPeerForTask(task *models.Task, peers []*GossipNode) *GossipNode {
	if len(peers) == 0 {
		return nil
	}
	
	switch gm.loadBalancingStrategy {
	case "least_loaded":
		return gm.selectLeastLoadedPeer(peers, task)
	case "lowest_latency":
		return gm.selectLowestLatencyPeer(peers)
	case "weighted":
		return gm.selectWeightedPeer(peers, task)
	default:
		return gm.selectLeastLoadedPeer(peers, task)
	}
}

// selectLeastLoadedPeer selects the peer with the lowest load
func (gm *GossipManager) selectLeastLoadedPeer(peers []*GossipNode, task *models.Task) *GossipNode {
	var bestPeer *GossipNode
	var lowestLoad float64 = 1.0
	
	for _, peer := range peers {
		if peer.Load < lowestLoad {
			lowestLoad = peer.Load
			bestPeer = peer
		}
	}
	
	return bestPeer
}

// selectLowestLatencyPeer selects the peer with the lowest latency
func (gm *GossipManager) selectLowestLatencyPeer(peers []*GossipNode) *GossipNode {
	var bestPeer *GossipNode
	var lowestLatency time.Duration = time.Hour
	
	for _, peer := range peers {
		if peer.Latency < lowestLatency {
			lowestLatency = peer.Latency
			bestPeer = peer
		}
	}
	
	return bestPeer
}

// selectWeightedPeer selects a peer using weighted scoring
func (gm *GossipManager) selectWeightedPeer(peers []*GossipNode, task *models.Task) *GossipNode {
	var bestPeer *GossipNode
	var bestScore float64 = -1
	
	for _, peer := range peers {
		// Calculate weighted score based on load, latency, and capacity
		loadScore := 1.0 - peer.Load
		latencyScore := 1.0 - (float64(peer.Latency) / float64(time.Second))
		
		// Normalize latency score
		if latencyScore < 0 {
			latencyScore = 0
		}
		
		// Weighted combination
		score := loadScore*0.6 + latencyScore*0.4
		
		if score > bestScore {
			bestScore = score
			bestPeer = peer
		}
	}
	
	return bestPeer
}

// sendTaskToPeer sends a task to a peer with retry logic
func (gm *GossipManager) sendTaskToPeer(task *models.Task, peer *GossipNode) bool {
	url := fmt.Sprintf("http://%s/api/v1/tasks", peer.Address)
	
	taskData := map[string]interface{}{
		"payload":    task.Payload,
		"parameters": task.Parameters,
		"priority":   task.Priority,
		"source_node": gm.nodeID,
		"redistributed": true,
	}
	
	jsonData, _ := json.Marshal(taskData)
	
	// Retry logic for task redistribution
	maxRetries := 3
	for attempt := 0; attempt < maxRetries; attempt++ {
		resp, err := gm.httpClient.Post(url, "application/json", bytes.NewBuffer(jsonData))
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return true
			}
		}
		
		// Wait before retry
		time.Sleep(time.Duration(attempt+1) * time.Second)
	}
	
	return false
}

// healthCheckLoop periodically performs health checks on peers
func (gm *GossipManager) healthCheckLoop() {
	ticker := time.NewTicker(gm.healthCheckInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-gm.ctx.Done():
			return
		case <-ticker.C:
			gm.performHealthChecks()
		}
	}
}

// performHealthChecks performs health checks on all peers
func (gm *GossipManager) performHealthChecks() {
	gm.mu.RLock()
	peers := make([]*GossipNode, 0, len(gm.peers))
	for _, peer := range gm.peers {
		peers = append(peers, peer)
	}
	gm.mu.RUnlock()
	
	for _, peer := range peers {
		go gm.checkPeerHealth(peer)
	}
}

// checkPeerHealth performs a health check on a specific peer
func (gm *GossipManager) checkPeerHealth(peer *GossipNode) {
	url := fmt.Sprintf("http://%s/health", peer.Address)
	
	start := time.Now()
	resp, err := gm.httpClient.Get(url)
	latency := time.Since(start)
	
	if err != nil {
		gm.markPeerSuspicious(peer.ID)
		return
	}
	defer resp.Body.Close()
	
	if resp.StatusCode == http.StatusOK {
		gm.markPeerAlive(peer.ID)
		gm.updatePeerLatency(peer.ID, latency)
	} else {
		gm.markPeerSuspicious(peer.ID)
	}
}

// latencyMonitoringLoop monitors and updates peer latencies
func (gm *GossipManager) latencyMonitoringLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-gm.ctx.Done():
			return
		case <-ticker.C:
			gm.updatePeerLatencies()
		}
	}
}

// updatePeerLatencies updates latency information for all peers
func (gm *GossipManager) updatePeerLatencies() {
	gm.mu.RLock()
	peers := make([]*GossipNode, 0, len(gm.peers))
	for _, peer := range gm.peers {
		peers = append(peers, peer)
	}
	gm.mu.RUnlock()
	
	for _, peer := range peers {
		go gm.measurePeerLatency(peer)
	}
}

// measurePeerLatency measures the latency to a specific peer
func (gm *GossipManager) measurePeerLatency(peer *GossipNode) {
	url := fmt.Sprintf("http://%s/health", peer.Address)
	
	start := time.Now()
	resp, err := gm.httpClient.Get(url)
	latency := time.Since(start)
	
	if err == nil && resp.StatusCode == http.StatusOK {
		resp.Body.Close()
		gm.updatePeerLatency(peer.ID, latency)
	}
}

// taskRedistributionLoop periodically checks for orphaned tasks
func (gm *GossipManager) taskRedistributionLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-gm.ctx.Done():
			return
		case <-ticker.C:
			gm.checkForOrphanedTasks()
		}
	}
}

// checkForOrphanedTasks checks for tasks that might be orphaned
func (gm *GossipManager) checkForOrphanedTasks() {
	// This would check for tasks that have been in the queue too long
	// and might need to be redistributed
	// Implementation depends on your task tracking mechanism
}

// HandleGossipMessage handles incoming gossip messages with validation
func (gm *GossipManager) HandleGossipMessage(message map[string]interface{}) {
	// Validate message structure
	nodeID, ok := message["node_id"].(string)
	if !ok {
		return
	}
	
	address, ok := message["address"].(string)
	if !ok {
		return
	}
	
	// Update peer information
	gm.mu.Lock()
	if peer, exists := gm.peers[nodeID]; exists {
		peer.LastSeen = time.Now()
		peer.Status = "alive"
		
		// Update additional information if available
		if load, ok := message["load"].(float64); ok {
			peer.Load = load
		}
		if latency, ok := message["latency"].(float64); ok {
			peer.Latency = time.Duration(latency)
		}
	} else {
		gm.peers[nodeID] = &GossipNode{
			ID:       nodeID,
			Address:  address,
			LastSeen: time.Now(),
			Status:   "alive",
			Load:     0.0,
			Latency:  0,
		}
	}
	gm.mu.Unlock()
	
	// Process peer information from the message
	if peersData, ok := message["peers"].(map[string]interface{}); ok {
		gm.updatePeerInformation(peersData)
	}
}

// updatePeerInformation updates peer information from gossip messages
func (gm *GossipManager) updatePeerInformation(peersData map[string]interface{}) {
	gm.mu.Lock()
	defer gm.mu.Unlock()
	
	for nodeID, peerData := range peersData {
		if peerDataMap, ok := peerData.(map[string]interface{}); ok {
			address := peerDataMap["address"].(string)
			lastSeen := time.Unix(int64(peerDataMap["last_seen"].(float64)), 0)
			status := peerDataMap["status"].(string)
			
			// Extract additional metrics
			load := 0.0
			if l, ok := peerDataMap["load"].(float64); ok {
				load = l
			}
			
			latency := time.Duration(0)
			if l, ok := peerDataMap["latency"].(float64); ok {
				latency = time.Duration(l)
			}
			
			if peer, exists := gm.peers[nodeID]; exists {
				// Update existing peer
				peer.LastSeen = lastSeen
				peer.Status = status
				peer.Load = load
				peer.Latency = latency
			} else {
				// Add new peer
				gm.peers[nodeID] = &GossipNode{
					ID:       nodeID,
					Address:  address,
					LastSeen: lastSeen,
					Status:   status,
					Load:     load,
					Latency:  latency,
				}
			}
		}
	}
} 