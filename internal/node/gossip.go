package node

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
	}
}

// Start begins the gossip protocol
func (gm *GossipManager) Start() {
	go gm.gossipLoop()
	go gm.failureDetectionLoop()
	go gm.taskRedistributionLoop()
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
	}
}

// RemovePeer removes a peer from the gossip network
func (gm *GossipManager) RemovePeer(nodeID string) {
	gm.mu.Lock()
	defer gm.mu.Unlock()
	
	delete(gm.peers, nodeID)
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
	
	// Select random peers to gossip with
	selectedPeers := gm.selectRandomPeers(peers, 3)
	
	for _, peer := range selectedPeers {
		go gm.sendGossipMessage(peer)
	}
}

// selectRandomPeers selects up to maxCount random peers
func (gm *GossipManager) selectRandomPeers(peers []*GossipNode, maxCount int) []*GossipNode {
	if len(peers) <= maxCount {
		return peers
	}
	
	// Simple random selection (in production, use crypto/rand)
	selected := make([]*GossipNode, 0, maxCount)
	used := make(map[int]bool)
	
	for len(selected) < maxCount {
		idx := int(time.Now().UnixNano()) % len(peers)
		if !used[idx] {
			selected = append(selected, peers[idx])
			used[idx] = true
		}
		time.Sleep(1 * time.Millisecond) // Small delay for different timestamps
	}
	
	return selected
}

// sendGossipMessage sends a gossip message to a peer
func (gm *GossipManager) sendGossipMessage(peer *GossipNode) {
	message := gm.createGossipMessage()
	
	url := fmt.Sprintf("http://%s/api/v1/gossip", peer.Address)
	resp, err := gm.httpClient.Post(url, "application/json", bytes.NewBuffer(message))
	if err != nil {
		gm.markPeerSuspicious(peer.ID)
		return
	}
	defer resp.Body.Close()
	
	if resp.StatusCode == http.StatusOK {
		gm.markPeerAlive(peer.ID)
	}
}

// createGossipMessage creates a gossip message
func (gm *GossipManager) createGossipMessage() []byte {
	message := map[string]interface{}{
		"node_id":  gm.nodeID,
		"address":  gm.address,
		"timestamp": time.Now().Unix(),
		"peers":    gm.GetPeers(),
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

// failureDetectionLoop periodically checks for failed nodes
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

// detectFailures checks for nodes that have failed
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
		}
	}
}

// handleNodeFailure handles the failure of a node
func (gm *GossipManager) handleNodeFailure(failedNodeID string) {
	// Try to redistribute tasks from the failed node
	gm.redistributeTasks(failedNodeID)
	
	// Remove the failed peer
	gm.RemovePeer(failedNodeID)
}

// redistributeTasks attempts to redistribute tasks from a failed node
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
	
	// Redistribute tasks
	for _, task := range tasks {
		// Find the best peer for this task
		bestPeer := gm.selectBestPeerForTask(task, availablePeers)
		if bestPeer != nil {
			gm.sendTaskToPeer(task, bestPeer)
		}
	}
}

// getTasksFromNode attempts to get tasks from a failed node
func (gm *GossipManager) getTasksFromNode(nodeID string) ([]*models.Task, error) {
	gm.mu.RLock()
	peer, exists := gm.peers[nodeID]
	gm.mu.RUnlock()
	
	if !exists {
		return nil, fmt.Errorf("peer not found")
	}
	
	// Try to get tasks from the failed node
	url := fmt.Sprintf("http://%s/api/v1/tasks/failed", peer.Address)
	resp, err := gm.httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get tasks from node")
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

// selectBestPeerForTask selects the best peer for a given task
func (gm *GossipManager) selectBestPeerForTask(task *models.Task, peers []*GossipNode) *GossipNode {
	if len(peers) == 0 {
		return nil
	}
	
	// Simple selection: pick the first available peer
	// In a real implementation, you'd check capacity and load
	return peers[0]
}

// sendTaskToPeer sends a task to a peer
func (gm *GossipManager) sendTaskToPeer(task *models.Task, peer *GossipNode) {
	url := fmt.Sprintf("http://%s/api/v1/tasks", peer.Address)
	
	taskData := map[string]interface{}{
		"payload":    task.Payload,
		"parameters": task.Parameters,
		"priority":   task.Priority,
		"source_node": gm.nodeID,
	}
	
	jsonData, _ := json.Marshal(taskData)
	resp, err := gm.httpClient.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return
	}
	defer resp.Body.Close()
	
	if resp.StatusCode == http.StatusOK {
		// Task successfully redistributed
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

// HandleGossipMessage handles incoming gossip messages
func (gm *GossipManager) HandleGossipMessage(message map[string]interface{}) {
	sourceNodeID := message["node_id"].(string)
	sourceAddress := message["address"].(string)
	
	// Update peer information
	gm.mu.Lock()
	if peer, exists := gm.peers[sourceNodeID]; exists {
		peer.LastSeen = time.Now()
		peer.Status = "alive"
	} else {
		gm.peers[sourceNodeID] = &GossipNode{
			ID:       sourceNodeID,
			Address:  sourceAddress,
			LastSeen: time.Now(),
			Status:   "alive",
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
			
			if peer, exists := gm.peers[nodeID]; exists {
				// Update existing peer
				peer.LastSeen = lastSeen
				peer.Status = status
			} else {
				// Add new peer
				gm.peers[nodeID] = &GossipNode{
					ID:       nodeID,
					Address:  address,
					LastSeen: lastSeen,
					Status:   status,
				}
			}
		}
	}
} 