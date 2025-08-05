package models

import (
	"time"
	"encoding/json"
)

// Task represents a single LLM processing request
type Task struct {
	ID              string                 `json:"id"`
	Payload         []byte                 `json:"payload"`
	Parameters      map[string]interface{} `json:"parameters"`
	Priority        int                    `json:"priority"`
	EstimatedTokens int                    `json:"estimated_tokens"`
	CreatedAt       time.Time             `json:"created_at"`
	StartedAt       *time.Time            `json:"started_at,omitempty"`
	CompletedAt     *time.Time            `json:"completed_at,omitempty"`
	Status          TaskStatus             `json:"status"`
	Result          *TaskResult            `json:"result,omitempty"`
	Error           string                 `json:"error,omitempty"`
	NodeID          string                 `json:"node_id,omitempty"`
	RetryCount      int                    `json:"retry_count"`
	MaxRetries      int                    `json:"max_retries"`
}

// TaskStatus represents the current state of a task
type TaskStatus string

const (
	TaskStatusPending   TaskStatus = "pending"
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusCompleted TaskStatus = "completed"
	TaskStatusFailed    TaskStatus = "failed"
	TaskStatusCancelled TaskStatus = "cancelled"
)

// TaskResult represents the output of a completed task
type TaskResult struct {
	Response    []byte                 `json:"response"`
	TokensUsed  int                    `json:"tokens_used"`
	ProcessingTime time.Duration       `json:"processing_time"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// NewTask creates a new task with default values
func NewTask(id string, payload []byte, parameters map[string]interface{}) *Task {
	return &Task{
		ID:              id,
		Payload:         payload,
		Parameters:      parameters,
		Priority:        0,
		EstimatedTokens: 0,
		CreatedAt:       time.Now(),
		Status:          TaskStatusPending,
		RetryCount:      0,
		MaxRetries:      3,
	}
}

// MarshalJSON implements custom JSON marshaling
func (t *Task) MarshalJSON() ([]byte, error) {
	type Alias Task
	return json.Marshal(&struct {
		*Alias
		CreatedAt   string  `json:"created_at"`
		StartedAt   *string `json:"started_at,omitempty"`
		CompletedAt *string `json:"completed_at,omitempty"`
	}{
		Alias:       (*Alias)(t),
		CreatedAt:   t.CreatedAt.Format(time.RFC3339),
		StartedAt:   formatTime(t.StartedAt),
		CompletedAt: formatTime(t.CompletedAt),
	})
}

func formatTime(t *time.Time) *string {
	if t == nil {
		return nil
	}
	formatted := t.Format(time.RFC3339)
	return &formatted
} 