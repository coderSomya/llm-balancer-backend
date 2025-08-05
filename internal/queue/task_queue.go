package queue

import (
	"container/heap"
	"context"
	"sync"
	"time"
	"llm-balancer/internal/models"
)

// TaskQueue is a thread-safe priority queue for tasks
type TaskQueue struct {
	items    *taskHeap
	capacity int
	mu       sync.RWMutex
	notify   chan struct{}
	ctx      context.Context
	cancel   context.CancelFunc
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
		items:    &taskHeap{},
		capacity: capacity,
		notify:   make(chan struct{}, 1),
		ctx:      ctx,
		cancel:   cancel,
	}
	
	heap.Init(queue.items)
	return queue
}

// Push adds a task to the queue
func (q *TaskQueue) Push(task *models.Task) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	
	if q.items.Len() >= q.capacity {
		return ErrQueueFull
	}
	
	heap.Push(q.items, task)
	
	// Notify waiting consumers
	select {
	case q.notify <- struct{}{}:
	default:
	}
	
	return nil
}

// Pop removes and returns the highest priority task
func (q *TaskQueue) Pop() (*models.Task, error) {
	return q.PopWithTimeout(0) // No timeout
}

// PopWithTimeout removes and returns the highest priority task with timeout
func (q *TaskQueue) PopWithTimeout(timeout time.Duration) (*models.Task, error) {
	for {
		q.mu.Lock()
		if q.items.Len() > 0 {
			task := heap.Pop(q.items).(*models.Task)
			q.mu.Unlock()
			return task, nil
		}
		q.mu.Unlock()
		
		// Wait for notification or timeout
		if timeout == 0 {
			select {
			case <-q.notify:
				continue
			case <-q.ctx.Done():
				return nil, ErrQueueClosed
			}
		} else {
			select {
			case <-q.notify:
				continue
			case <-time.After(timeout):
				return nil, ErrTimeout
			case <-q.ctx.Done():
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
		return nil, ErrQueueEmpty
	}
	
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

// RemoveTask removes a specific task by ID
func (q *TaskQueue) RemoveTask(taskID string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	
	for i, task := range *q.items {
		if task.ID == taskID {
			heap.Remove(q.items, i)
			return true
		}
	}
	
	return false
}

// UpdateTask updates a task in the queue
func (q *TaskQueue) UpdateTask(taskID string, updater func(*models.Task)) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	
	for i, task := range *q.items {
		if task.ID == taskID {
			updater(task)
			heap.Fix(q.items, i)
			return true
		}
	}
	
	return false
}

// Close closes the queue and cancels all waiting operations
func (q *TaskQueue) Close() {
	q.cancel()
	close(q.notify)
}

// GetStats returns queue statistics
func (q *TaskQueue) GetStats() QueueStats {
	q.mu.RLock()
	defer q.mu.RUnlock()
	
	stats := QueueStats{
		Size:     q.items.Len(),
		Capacity: q.capacity,
		Status:   make(map[models.TaskStatus]int),
	}
	
	for _, task := range *q.items {
		stats.Status[task.Status]++
	}
	
	return stats
}

// QueueStats represents queue statistics
type QueueStats struct {
	Size     int                           `json:"size"`
	Capacity int                           `json:"capacity"`
	Status   map[models.TaskStatus]int     `json:"status"`
} 