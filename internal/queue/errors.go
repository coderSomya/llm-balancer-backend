package queue

import "errors"

var (
	// ErrQueueEmpty is returned when trying to pop from an empty queue
	ErrQueueEmpty = errors.New("queue is empty")
	
	// ErrQueueFull is returned when trying to push to a full queue
	ErrQueueFull = errors.New("queue is full")
	
	// ErrQueueClosed is returned when the queue is closed
	ErrQueueClosed = errors.New("queue is closed")
	
	// ErrTimeout is returned when a timeout occurs
	ErrTimeout = errors.New("operation timed out")
	
	// ErrTaskNotFound is returned when a task is not found in the queue
	ErrTaskNotFound = errors.New("task not found")
	
	// ErrInvalidTask is returned when an invalid task is provided
	ErrInvalidTask = errors.New("invalid task")
) 