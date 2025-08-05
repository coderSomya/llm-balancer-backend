package queue

import (
	"container/heap"
	"context"
	"fmt"
	"sync"
	"time"
	"llm-balancer/internal/models"
)

// TaskQueue is a thread-safe priority queue for tasks with advanced features
type TaskQueue struct {
	items    *taskHeap
	capacity int
	mu       sync.RWMutex
	notify   chan struct{}
	ctx      context.Context
	cancel   context.CancelFunc
	
	// Production features
	metrics           map[string]interface{}
	lastOperationTime time.Time
	operationCounts   map[string]int
	errorCounts       map[string]int
	avgWaitTime       time.Duration
	peakSize          int
}

// taskHeap implements heap.Interface for priority queue
type taskHeap []*models.Task

func (h taskHeap) Len() int { return len(h) }

func (h taskHeap) Less(i, j int) bool {
	// Higher priority first, then by creation time (FIFO for same priority)
	if h[i].Priority != h[j].Priority {
		return h[i].Priority > h[j].Priority
	}
	return h[i].CreatedAt.Before(h[j].CreatedAt)
}

func (h taskHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}

func (h taskHeap) Push(x interface{}) {
	item := x.(*models.Task)
	h = append(h, item)
}

func (h taskHeap) Pop() interface{} {
	old := h
	n := len(old)
	item := old[n-1]
	h = old[0 : n-1]
	return item
}

// NewTaskQueue creates a new task queue with specified capacity
func NewTaskQueue(capacity int) *TaskQueue {
	ctx, cancel := context.WithCancel(context.Background())
	
	queue := &TaskQueue{
		items:            &taskHeap{},
		capacity:         capacity,
		notify:           make(chan struct{}, 1),
		ctx:              ctx,
		cancel:           cancel,
		metrics:          make(map[string]interface{}),
		operationCounts:  make(map[string]int),
		errorCounts:      make(map[string]int),
	}
	
	heap.Init(queue.items)
	return queue
}

// Push adds a task to the queue with enhanced error handling
func (q *TaskQueue) Push(task *models.Task) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	
	// Validate task
	if task == nil {
		q.recordError("push", "nil_task")
		return ErrInvalidTask
	}
	
	if len(task.Payload) == 0 {
		q.recordError("push", "empty_payload")
		return ErrInvalidTask
	}
	
	// Check capacity
	if q.items.Len() >= q.capacity {
		q.recordError("push", "queue_full")
		return ErrQueueFull
	}
	
	// Add task to heap
	heap.Push(q.items, task)
	
	// Update metrics
	q.recordOperation("push")
	q.updatePeakSize()
	q.updateLastOperationTime()
	
	// Notify waiting consumers
	select {
	case q.notify <- struct{}{}:
	default:
	}
	
	return nil
}

// Pop removes and returns the highest priority task with enhanced error handling
func (q *TaskQueue) Pop() (*models.Task, error) {
	return q.PopWithTimeout(0) // No timeout
}

// PopWithTimeout removes and returns the highest priority task with timeout
func (q *TaskQueue) PopWithTimeout(timeout time.Duration) (*models.Task, error) {
	startTime := time.Now()
	
	for {
		q.mu.Lock()
		if q.items.Len() > 0 {
			task := heap.Pop(q.items).(*models.Task)
			q.mu.Unlock()
			
			// Update metrics
			q.recordOperation("pop")
			q.updateWaitTime(time.Since(startTime))
			q.updateLastOperationTime()
			
			return task, nil
		}
		q.mu.Unlock()
		
		// Wait for notification or timeout
		if timeout == 0 {
			select {
			case <-q.notify:
				continue
			case <-q.ctx.Done():
				q.recordError("pop", "context_cancelled")
				return nil, ErrQueueClosed
			}
		} else {
			select {
			case <-q.notify:
				continue
			case <-time.After(timeout):
				q.recordError("pop", "timeout")
				return nil, ErrTimeout
			case <-q.ctx.Done():
				q.recordError("pop", "context_cancelled")
				return nil, ErrQueueClosed
			}
		}
	}
}

// Peek returns the highest priority task without removing it
func (q *TaskQueue) Peek() (*models.Task, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()
	
	if q.items.Len() == 0 {
		q.recordError("peek", "queue_empty")
		return nil, ErrQueueEmpty
	}
	
	q.recordOperation("peek")
	return (*q.items)[0], nil
}

// Size returns the current number of items in the queue
func (q *TaskQueue) Size() int {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.items.Len()
}

// IsEmpty returns true if the queue is empty
func (q *TaskQueue) IsEmpty() bool {
	return q.Size() == 0
}

// IsFull returns true if the queue is at capacity
func (q *TaskQueue) IsFull() bool {
	return q.Size() >= q.capacity
}

// Clear removes all tasks from the queue
func (q *TaskQueue) Clear() {
	q.mu.Lock()
	defer q.mu.Unlock()
	
	// Reset the heap
	q.items = &taskHeap{}
	heap.Init(q.items)
	
	// Update metrics
	q.recordOperation("clear")
	q.updateLastOperationTime()
}

// GetTasksByStatus returns all tasks with the specified status
func (q *TaskQueue) GetTasksByStatus(status models.TaskStatus) []*models.Task {
	q.mu.RLock()
	defer q.mu.RUnlock()
	
	var tasks []*models.Task
	for _, task := range *q.items {
		if task.Status == status {
			tasks = append(tasks, task)
		}
	}
	
	return tasks
}

// RemoveTask removes a specific task by ID with enhanced error handling
func (q *TaskQueue) RemoveTask(taskID string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	
	for i, task := range *q.items {
		if task.ID == taskID {
			heap.Remove(q.items, i)
			q.recordOperation("remove")
			q.updateLastOperationTime()
			return true
		}
	}
	
	q.recordError("remove", "task_not_found")
	return false
}

// UpdateTask updates a task in the queue with enhanced error handling
func (q *TaskQueue) UpdateTask(taskID string, updater func(*models.Task)) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	
	for i, task := range *q.items {
		if task.ID == taskID {
			updater(task)
			heap.Fix(q.items, i)
			q.recordOperation("update")
			q.updateLastOperationTime()
			return true
		}
	}
	
	q.recordError("update", "task_not_found")
	return false
}

// Close closes the queue and cancels all waiting operations
func (q *TaskQueue) Close() {
	q.cancel()
	close(q.notify)
}

// GetStats returns comprehensive queue statistics
func (q *TaskQueue) GetStats() QueueStats {
	q.mu.RLock()
	defer q.mu.RUnlock()
	
	stats := QueueStats{
		Size:     q.items.Len(),
		Capacity: q.capacity,
		Status:   make(map[models.TaskStatus]int),
		Metrics:  q.getMetrics(),
	}
	
	for _, task := range *q.items {
		stats.Status[task.Status]++
	}
	
	return stats
}

// QueueStats represents comprehensive queue statistics
type QueueStats struct {
	Size     int                           `json:"size"`
	Capacity int                           `json:"capacity"`
	Status   map[models.TaskStatus]int     `json:"status"`
	Metrics  map[string]interface{}        `json:"metrics"`
}

// recordOperation records an operation for metrics
func (q *TaskQueue) recordOperation(operation string) {
	q.operationCounts[operation]++
}

// recordError records an error for metrics
func (q *TaskQueue) recordError(operation, errorType string) {
	key := fmt.Sprintf("%s_%s", operation, errorType)
	q.errorCounts[key]++
}

// updatePeakSize updates the peak size metric
func (q *TaskQueue) updatePeakSize() {
	currentSize := q.items.Len()
	if currentSize > q.peakSize {
		q.peakSize = currentSize
	}
}

// updateLastOperationTime updates the last operation time
func (q *TaskQueue) updateLastOperationTime() {
	q.lastOperationTime = time.Now()
}

// updateWaitTime updates the average wait time
func (q *TaskQueue) updateWaitTime(waitTime time.Duration) {
	if q.avgWaitTime == 0 {
		q.avgWaitTime = waitTime
	} else {
		q.avgWaitTime = (q.avgWaitTime + waitTime) / 2
	}
}

// getMetrics returns comprehensive metrics
func (q *TaskQueue) getMetrics() map[string]interface{} {
	utilization := 0.0
	if q.capacity > 0 {
		utilization = float64(q.items.Len()) / float64(q.capacity) * 100
	}
	
	return map[string]interface{}{
		"utilization_percentage": utilization,
		"peak_size":             q.peakSize,
		"avg_wait_time":         q.avgWaitTime,
		"last_operation_time":   q.lastOperationTime,
		"operation_counts":      q.operationCounts,
		"error_counts":          q.errorCounts,
		"is_empty":             q.items.Len() == 0,
		"is_full":              q.items.Len() >= q.capacity,
	}
}

// GetPriorityDistribution returns the distribution of task priorities
func (q *TaskQueue) GetPriorityDistribution() map[int]int {
	q.mu.RLock()
	defer q.mu.RUnlock()
	
	distribution := make(map[int]int)
	for _, task := range *q.items {
		distribution[task.Priority]++
	}
	
	return distribution
}

// GetTasksByPriority returns all tasks with the specified priority
func (q *TaskQueue) GetTasksByPriority(priority int) []*models.Task {
	q.mu.RLock()
	defer q.mu.RUnlock()
	
	var tasks []*models.Task
	for _, task := range *q.items {
		if task.Priority == priority {
			tasks = append(tasks, task)
		}
	}
	
	return tasks
}

// GetOldestTask returns the oldest task in the queue
func (q *TaskQueue) GetOldestTask() (*models.Task, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()
	
	if q.items.Len() == 0 {
		return nil, ErrQueueEmpty
	}
	
	var oldestTask *models.Task
	var oldestTime time.Time
	
	for _, task := range *q.items {
		if oldestTask == nil || task.CreatedAt.Before(oldestTime) {
			oldestTask = task
			oldestTime = task.CreatedAt
		}
	}
	
	return oldestTask, nil
}

// GetNewestTask returns the newest task in the queue
func (q *TaskQueue) GetNewestTask() (*models.Task, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()
	
	if q.items.Len() == 0 {
		return nil, ErrQueueEmpty
	}
	
	var newestTask *models.Task
	var newestTime time.Time
	
	for _, task := range *q.items {
		if newestTask == nil || task.CreatedAt.After(newestTime) {
			newestTask = task
			newestTime = task.CreatedAt
		}
	}
	
	return newestTask, nil
}

// GetTasksOlderThan returns tasks older than the specified duration
func (q *TaskQueue) GetTasksOlderThan(duration time.Duration) []*models.Task {
	q.mu.RLock()
	defer q.mu.RUnlock()
	
	var oldTasks []*models.Task
	cutoff := time.Now().Add(-duration)
	
	for _, task := range *q.items {
		if task.CreatedAt.Before(cutoff) {
			oldTasks = append(oldTasks, task)
		}
	}
	
	return oldTasks
}

// GetTasksBySize returns tasks grouped by payload size ranges
func (q *TaskQueue) GetTasksBySize() map[string]int {
	q.mu.RLock()
	defer q.mu.RUnlock()
	
	sizeRanges := map[string]int{
		"small":  0, // < 1KB
		"medium": 0, // 1KB - 10KB
		"large":  0, // > 10KB
	}
	
	for _, task := range *q.items {
		size := len(task.Payload)
		switch {
		case size < 1024:
			sizeRanges["small"]++
		case size < 10*1024:
			sizeRanges["medium"]++
		default:
			sizeRanges["large"]++
		}
	}
	
	return sizeRanges
}

// Drain removes all tasks from the queue and returns them
func (q *TaskQueue) Drain() []*models.Task {
	q.mu.Lock()
	defer q.mu.Unlock()
	
	var tasks []*models.Task
	for q.items.Len() > 0 {
		task := heap.Pop(q.items).(*models.Task)
		tasks = append(tasks, task)
	}
	
	q.recordOperation("drain")
	q.updateLastOperationTime()
	
	return tasks
}

// IsHealthy returns true if the queue is in a healthy state
func (q *TaskQueue) IsHealthy() bool {
	q.mu.RLock()
	defer q.mu.RUnlock()
	
	// Check if queue is not too full
	if q.items.Len() > q.capacity*9/10 {
		return false
	}
	
	// Check if there are too many errors
	totalErrors := 0
	for _, count := range q.errorCounts {
		totalErrors += count
	}
	
	totalOperations := 0
	for _, count := range q.operationCounts {
		totalOperations += count
	}
	
	if totalOperations > 0 {
		errorRate := float64(totalErrors) / float64(totalOperations)
		if errorRate > 0.1 { // 10% error rate threshold
			return false
		}
	}
	
	return true
} 