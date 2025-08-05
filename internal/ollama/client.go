package ollama

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// OllamaClient handles communication with Ollama API
type OllamaClient struct {
	baseURL    string
	httpClient *http.Client
}

// OllamaRequest represents a request to Ollama
type OllamaRequest struct {
	Model       string                 `json:"model"`
	Prompt      string                 `json:"prompt"`
	Stream      bool                   `json:"stream"`
	Options     map[string]interface{} `json:"options,omitempty"`
	Temperature float64                `json:"temperature,omitempty"`
	MaxTokens   int                    `json:"max_tokens,omitempty"`
}

// OllamaResponse represents a response from Ollama
type OllamaResponse struct {
	Model     string `json:"model"`
	CreatedAt string `json:"created_at"`
	Response  string `json:"response"`
	Done      bool   `json:"done"`
	Context   []int  `json:"context,omitempty"`
	TotalDuration int64 `json:"total_duration,omitempty"`
	LoadDuration int64 `json:"load_duration,omitempty"`
	PromptEvalCount int `json:"prompt_eval_count,omitempty"`
	PromptEvalDuration int64 `json:"prompt_eval_duration,omitempty"`
	EvalCount int `json:"eval_count,omitempty"`
	EvalDuration int64 `json:"eval_duration,omitempty"`
}

// NewOllamaClient creates a new Ollama client
func NewOllamaClient(baseURL string) *OllamaClient {
	return &OllamaClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 5 * time.Minute, // Ollama can take time to respond
		},
	}
}

// Generate sends a request to Ollama and returns the response
func (c *OllamaClient) Generate(prompt string, parameters map[string]interface{}) (*OllamaResponse, error) {
	// Extract parameters
	model := "qwen2.5"
	if m, ok := parameters["model"].(string); ok && m != "" {
		model = m
	}
	
	temperature := 0.7
	if t, ok := parameters["temperature"].(float64); ok {
		temperature = t
	}
	
	maxTokens := 500
	if mt, ok := parameters["max_tokens"].(int); ok && mt > 0 {
		maxTokens = mt
	}
	
	// Create request
	req := OllamaRequest{
		Model:       model,
		Prompt:      prompt,
		Stream:      false,
		Temperature: temperature,
		MaxTokens:   maxTokens,
	}
	
	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %v", err)
	}
	
	// Send request
	resp, err := c.httpClient.Post(c.baseURL+"/api/generate", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to send request to Ollama: %v", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Ollama returned status %d: %s", resp.StatusCode, string(body))
	}
	
	// Parse response
	var ollamaResp OllamaResponse
	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		return nil, fmt.Errorf("failed to decode Ollama response: %v", err)
	}
	
	return &ollamaResp, nil
}

// ListModels returns available models
func (c *OllamaClient) ListModels() ([]string, error) {
	resp, err := c.httpClient.Get(c.baseURL + "/api/tags")
	if err != nil {
		return nil, fmt.Errorf("failed to get models: %v", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Ollama returned status %d", resp.StatusCode)
	}
	
	var response struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode models response: %v", err)
	}
	
	var models []string
	for _, model := range response.Models {
		models = append(models, model.Name)
	}
	
	return models, nil
}

// HealthCheck checks if Ollama is running
func (c *OllamaClient) HealthCheck() error {
	resp, err := c.httpClient.Get(c.baseURL + "/api/tags")
	if err != nil {
		return fmt.Errorf("Ollama health check failed: %v", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Ollama health check returned status %d", resp.StatusCode)
	}
	
	return nil
} 