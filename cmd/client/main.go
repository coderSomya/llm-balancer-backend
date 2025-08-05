package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type TaskRequest struct {
	Payload    string                 `json:"payload"`
	Parameters map[string]interface{} `json:"parameters"`
	Priority   int                    `json:"priority"`
}

type TaskResponse struct {
	TaskID   string `json:"task_id"`
	NodeID   string `json:"node_id"`
	Status   string `json:"status"`
	Message  string `json:"message,omitempty"`
}

type TaskInfo struct {
	TaskID      string                 `json:"task_id"`
	NodeID      string                 `json:"node_id"`
	Status      string                 `json:"status"`
	CreatedAt   time.Time              `json:"created_at"`
	LastUpdated time.Time              `json:"last_updated"`
	Payload     []byte                 `json:"payload"`
	Parameters  map[string]interface{} `json:"parameters"`
	Priority    int                    `json:"priority"`
	Result      interface{}            `json:"result,omitempty"`
	Error       string                 `json:"error,omitempty"`
}

func main() {
	fmt.Println("🚀 LLM Balancer Test Client")
	fmt.Println("=============================")

	// Test messages
	testMessages := []string{
		"Hello, how are you today?",
		"What is the capital of France?",
		"Explain quantum computing in simple terms",
		"Write a short poem about technology",
		"What are the benefits of renewable energy?",
	}

	gatewayURL := "http://localhost:8081"
	
	fmt.Printf("📡 Connecting to gateway at: %s\n", gatewayURL)
	
	// Test gateway health first
	if !testGatewayHealth(gatewayURL) {
		fmt.Println("❌ Gateway is not responding. Please start the gateway first.")
		return
	}
	
	fmt.Println("✅ Gateway is healthy!")
	
	// Submit tasks
	var taskIDs []string
	for i, message := range testMessages {
		fmt.Printf("\n📤 Submitting task %d: %s\n", i+1, truncateString(message, 50))
		
		taskID, err := submitTask(gatewayURL, message)
		if err != nil {
			fmt.Printf("❌ Failed to submit task: %v\n", err)
			continue
		}
		
		taskIDs = append(taskIDs, taskID)
		fmt.Printf("✅ Task submitted successfully. Task ID: %s\n", taskID)
	}
	
	// Monitor task completion
	fmt.Println("\n🔄 Monitoring task completion...")
	monitorTasks(gatewayURL, taskIDs)
}

func testGatewayHealth(gatewayURL string) bool {
	resp, err := http.Get(gatewayURL + "/api/v1/health")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func submitTask(gatewayURL, message string) (string, error) {
	req := TaskRequest{
		Payload: message,
		Parameters: map[string]interface{}{
			"model":       "qwen2.5",
			"temperature": 0.7,
			"max_tokens":  500,
		},
		Priority: 5,
	}
	
	jsonData, err := json.Marshal(req)
	if err != nil {
		return "", err
	}
	
	resp, err := http.Post(gatewayURL+"/api/v1/tasks", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("gateway returned status %d: %s", resp.StatusCode, string(body))
	}
	
	var taskResp TaskResponse
	if err := json.NewDecoder(resp.Body).Decode(&taskResp); err != nil {
		return "", err
	}
	
	return taskResp.TaskID, nil
}

func monitorTasks(gatewayURL string, taskIDs []string) {
	maxAttempts := 30 // 5 minutes with 10-second intervals
	attempt := 0
	
	for attempt < maxAttempts {
		attempt++
		fmt.Printf("\n📊 Checking task status (attempt %d/%d)...\n", attempt, maxAttempts)
		
		completed := 0
		failed := 0
		
		for _, taskID := range taskIDs {
			status, result, err := getTaskStatus(gatewayURL, taskID)
			if err != nil {
				fmt.Printf("❌ Error checking task %s: %v\n", taskID, err)
				continue
			}
			
			switch status {
			case "completed":
				completed++
				fmt.Printf("✅ Task %s completed!\n", taskID)
				if result != nil {
					fmt.Printf("📝 Result: %s\n", truncateString(fmt.Sprintf("%v", result), 100))
				}
			case "failed":
				failed++
				fmt.Printf("❌ Task %s failed\n", taskID)
			case "running":
				fmt.Printf("🔄 Task %s is running...\n", taskID)
			case "submitted":
				fmt.Printf("📤 Task %s is submitted...\n", taskID)
			default:
				fmt.Printf("⏳ Task %s status: %s\n", taskID, status)
			}
		}
		
		if completed == len(taskIDs) {
			fmt.Printf("\n🎉 All %d tasks completed successfully!\n", completed)
			return
		}
		
		if failed > 0 {
			fmt.Printf("\n⚠️  %d tasks failed, %d completed, %d still running\n", failed, completed, len(taskIDs)-completed-failed)
		} else {
			fmt.Printf("\n📈 Progress: %d completed, %d still running\n", completed, len(taskIDs)-completed)
		}
		
		time.Sleep(10 * time.Second)
	}
	
	fmt.Println("\n⏰ Timeout reached. Some tasks may still be processing.")
}

func getTaskStatus(gatewayURL, taskID string) (string, interface{}, error) {
	resp, err := http.Get(gatewayURL + "/api/v1/tasks/" + taskID)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("gateway returned status %d", resp.StatusCode)
	}
	
	var taskInfo TaskInfo
	if err := json.NewDecoder(resp.Body).Decode(&taskInfo); err != nil {
		return "", nil, err
	}
	
	return taskInfo.Status, taskInfo.Result, nil
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
} 