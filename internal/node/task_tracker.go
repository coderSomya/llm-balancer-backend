package node

import (
	"sync"
	"time"
	"llm-balancer/internal/models"
)

// TaskTracker tracks tasks and their states for failure recovery
type TaskTracker struct {
	tasks       map[string]*models.Task
	mu          sync.RWMutex
	maxTaskAge  time.Duration
	cleanupInterval time.Duration
}

// NewTaskTracker creates a new task tracker
func NewTaskTracker() *TaskTracker {
	tracker := &TaskTracker{
		tasks:           make(map[string]*models.Task),
		maxTaskAge:      24 * time.Hour, // Keep tasks for 24 hours
		cleanupInterval: 1 * time.Hour,  // Cleanup every hour
	}
	
	// Start cleanup routine
	go tracker.cleanupLoop()
	
	return tracker
}

// AddTask adds a task to the tracker
func (tt *TaskTracker) AddTask(task *models.Task) {
	tt.mu.Lock()
	defer tt.mu.Unlock()
	
	tt.tasks[task.ID] = task
}

// UpdateTask updates a task in the tracker
func (tt *TaskTracker) UpdateTask(taskID string, updater func(*models.Task)) {
	tt.mu.Lock()
	defer tt.mu.Unlock()
	
	if task, exists := tt.tasks[taskID]; exists {
		updater(task)
	}
}

// GetTask returns a task by ID
func (tt *TaskTracker) GetTask(taskID string) (*models.Task, bool) {
	tt.mu.RLock()
	defer tt.mu.RUnlock()
	
	task, exists := tt.tasks[taskID]
	return task, exists
}

// GetTasksByStatus returns all tasks with the specified status
func (tt *TaskTracker) GetTasksByStatus(status models.TaskStatus) []*models.Task {
	tt.mu.RLock()
	defer tt.mu.RUnlock()
	
	var tasks []*models.Task
	for _, task := range tt.tasks {
		if task.Status == status {
			tasks = append(tasks, task)
		}
	}
	return tasks
}

// GetFailedTasks returns all failed tasks that can be redistributed
func (tt *TaskTracker) GetFailedTasks() []*models.Task {
	tt.mu.RLock()
	defer tt.mu.RUnlock()
	
	var failedTasks []*models.Task
	now := time.Now()
	
	for _, task := range tt.tasks {
		// Consider tasks as failed if they've been running too long
		// or if they're in a failed state
		if task.Status == models.TaskStatusFailed {
			failedTasks = append(failedTasks, task)
		} else if task.Status == models.TaskStatusRunning {
			// Check if task has been running too long
			if task.StartedAt != nil && now.Sub(*task.StartedAt) > 5*time.Minute {
				failedTasks = append(failedTasks, task)
			}
		}
	}
	
	return failedTasks
}

// GetOrphanedTasks returns tasks that might be orphaned
func (tt *TaskTracker) GetOrphanedTasks() []*models.Task {
	tt.mu.RLock()
	defer tt.mu.RUnlock()
	
	var orphanedTasks []*models.Task
	now := time.Now()
	
	for _, task := range tt.tasks {
		// Tasks are considered orphaned if they've been pending too long
		if task.Status == models.TaskStatusPending {
			if now.Sub(task.CreatedAt) > 10*time.Minute {
				orphanedTasks = append(orphanedTasks, task)
			}
		}
	}
	
	return orphanedTasks
}

// RemoveTask removes a task from the tracker
func (tt *TaskTracker) RemoveTask(taskID string) {
	tt.mu.Lock()
	defer tt.mu.Unlock()
	
	delete(tt.tasks, taskID)
}

// GetStats returns tracker statistics
func (tt *TaskTracker) GetStats() map[string]interface{} {
	tt.mu.RLock()
	defer tt.mu.RUnlock()
	
	stats := map[string]interface{}{
		"total_tasks": len(tt.tasks),
		"by_status":   make(map[models.TaskStatus]int),
		"by_age":      make(map[string]int),
	}
	
	now := time.Now()
	
	for _, task := range tt.tasks {
		// Count by status
		if count, exists := stats["by_status"].(map[models.TaskStatus]int); exists {
			count[task.Status]++
		}
		
		// Count by age
		age := now.Sub(task.CreatedAt)
		ageGroup := "recent"
		if age > 1*time.Hour {
			ageGroup = "old"
		} else if age > 10*time.Minute {
			ageGroup = "medium"
		}
		
		if ageCount, exists := stats["by_age"].(map[string]int); exists {
			ageCount[ageGroup]++
		}
	}
	
	return stats
}

// cleanupLoop periodically cleans up old tasks
func (tt *TaskTracker) cleanupLoop() {
	ticker := time.NewTicker(tt.cleanupInterval)
	defer ticker.Stop()
	
	for range ticker.C {
		tt.cleanup()
	}
}

// cleanup removes old tasks from the tracker
func (tt *TaskTracker) cleanup() {
	tt.mu.Lock()
	defer tt.mu.Unlock()
	
	now := time.Now()
	
	for taskID, task := range tt.tasks {
		// Remove tasks older than maxTaskAge
		if now.Sub(task.CreatedAt) > tt.maxTaskAge {
			delete(tt.tasks, taskID)
		}
	}
}

// MarkTaskForRedistribution marks a task for redistribution
func (tt *TaskTracker) MarkTaskForRedistribution(taskID string) {
	tt.UpdateTask(taskID, func(task *models.Task) {
		task.Status = models.TaskStatusPending
		task.StartedAt = nil
		task.CompletedAt = nil
		task.Result = nil
		task.Error = ""
	})
}

// GetTasksForRedistribution returns tasks that can be redistributed
func (tt *TaskTracker) GetTasksForRedistribution() []*models.Task {
	// Get failed and orphaned tasks
	failedTasks := tt.GetFailedTasks()
	orphanedTasks := tt.GetOrphanedTasks()
	
	// Combine and deduplicate
	taskMap := make(map[string]*models.Task)
	
	for _, task := range failedTasks {
		taskMap[task.ID] = task
	}
	
	for _, task := range orphanedTasks {
		taskMap[task.ID] = task
	}
	
	// Convert back to slice
	var tasks []*models.Task
	for _, task := range taskMap {
		tasks = append(tasks, task)
	}
	
	return tasks
} 