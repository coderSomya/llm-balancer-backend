package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
	
	"github.com/spf13/viper"
)

// Config holds all configuration for the LLM balancer
type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Node     NodeConfig     `mapstructure:"node"`
	Gateway  GatewayConfig  `mapstructure:"gateway"`
	Queue    QueueConfig    `mapstructure:"queue"`
	Logging  LoggingConfig  `mapstructure:"logging"`
	Metrics  MetricsConfig  `mapstructure:"metrics"`
}

// ServerConfig holds server-related configuration
type ServerConfig struct {
	Host string `mapstructure:"host"`
	Port int    `mapstructure:"port"`
}

// NodeConfig holds node-specific configuration
type NodeConfig struct {
	ID                    string        `mapstructure:"id"`
	Address               string        `mapstructure:"address"`
	MaxRequestsPerMinute  int           `mapstructure:"max_requests_per_minute"`
	MaxTokensPerMinute    int           `mapstructure:"max_tokens_per_minute"`
	MaxConcurrentTasks    int           `mapstructure:"max_concurrent_tasks"`
	MaxQueueSize          int           `mapstructure:"max_queue_size"`
	WorkerPoolSize        int           `mapstructure:"worker_pool_size"`
	HeartbeatInterval     time.Duration `mapstructure:"heartbeat_interval"`
	HealthCheckTimeout    time.Duration `mapstructure:"health_check_timeout"`
}

// GatewayConfig holds gateway-specific configuration
type GatewayConfig struct {
	Host                  string        `mapstructure:"host"`
	Port                  int           `mapstructure:"port"`
	NodeDiscoveryInterval time.Duration `mapstructure:"node_discovery_interval"`
	LoadBalancingStrategy string        `mapstructure:"load_balancing_strategy"`
	CircuitBreakerEnabled bool          `mapstructure:"circuit_breaker_enabled"`
	CircuitBreakerThreshold int         `mapstructure:"circuit_breaker_threshold"`
}

// QueueConfig holds queue-related configuration
type QueueConfig struct {
	Capacity              int           `mapstructure:"capacity"`
	PriorityEnabled       bool          `mapstructure:"priority_enabled"`
	Timeout               time.Duration `mapstructure:"timeout"`
	RetryAttempts         int           `mapstructure:"retry_attempts"`
	RetryDelay            time.Duration `mapstructure:"retry_delay"`
}

// LoggingConfig holds logging configuration
type LoggingConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
	Output string `mapstructure:"output"`
}

// MetricsConfig holds metrics configuration
type MetricsConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Port    int    `mapstructure:"port"`
	Path    string `mapstructure:"path"`
}

// LoadConfig loads configuration from file and environment variables
func LoadConfig(configPath string) (*Config, error) {
	viper.SetConfigFile(configPath)
	viper.AutomaticEnv()
	
	// Set default values
	setDefaults()
	
	// Read config file
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
	}
	
	var config Config
	if err := viper.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}
	
	// Validate configuration
	if err := validateConfig(&config); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}
	
	return &config, nil
}

// setDefaults sets default configuration values
func setDefaults() {
	// Server defaults
	viper.SetDefault("server.host", "0.0.0.0")
	viper.SetDefault("server.port", 8080)
	
	// Node defaults
	viper.SetDefault("node.id", getEnvOrDefault("NODE_ID", "node-1"))
	viper.SetDefault("node.address", getEnvOrDefault("NODE_ADDRESS", "localhost:8081"))
	viper.SetDefault("node.max_requests_per_minute", 100)
	viper.SetDefault("node.max_tokens_per_minute", 10000)
	viper.SetDefault("node.max_concurrent_tasks", 10)
	viper.SetDefault("node.max_queue_size", 1000)
	viper.SetDefault("node.worker_pool_size", 5)
	viper.SetDefault("node.heartbeat_interval", "30s")
	viper.SetDefault("node.health_check_timeout", "10s")
	
	// Gateway defaults
	viper.SetDefault("gateway.host", "0.0.0.0")
	viper.SetDefault("gateway.port", 8080)
	viper.SetDefault("gateway.node_discovery_interval", "60s")
	viper.SetDefault("gateway.load_balancing_strategy", "round_robin")
	viper.SetDefault("gateway.circuit_breaker_enabled", true)
	viper.SetDefault("gateway.circuit_breaker_threshold", 5)
	
	// Queue defaults
	viper.SetDefault("queue.capacity", 1000)
	viper.SetDefault("queue.priority_enabled", true)
	viper.SetDefault("queue.timeout", "30s")
	viper.SetDefault("queue.retry_attempts", 3)
	viper.SetDefault("queue.retry_delay", "5s")
	
	// Logging defaults
	viper.SetDefault("logging.level", "info")
	viper.SetDefault("logging.format", "json")
	viper.SetDefault("logging.output", "stdout")
	
	// Metrics defaults
	viper.SetDefault("metrics.enabled", true)
	viper.SetDefault("metrics.port", 9090)
	viper.SetDefault("metrics.path", "/metrics")
}

// getEnvOrDefault gets an environment variable or returns a default value
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// validateConfig validates the configuration
func validateConfig(config *Config) error {
	if config.Node.MaxRequestsPerMinute <= 0 {
		return fmt.Errorf("max_requests_per_minute must be positive")
	}
	
	if config.Node.MaxTokensPerMinute <= 0 {
		return fmt.Errorf("max_tokens_per_minute must be positive")
	}
	
	if config.Node.MaxConcurrentTasks <= 0 {
		return fmt.Errorf("max_concurrent_tasks must be positive")
	}
	
	if config.Node.MaxQueueSize <= 0 {
		return fmt.Errorf("max_queue_size must be positive")
	}
	
	if config.Queue.Capacity <= 0 {
		return fmt.Errorf("queue capacity must be positive")
	}
	
	return nil
}

// GetNodeID returns the node ID, generating one if not set
func GetNodeID() string {
	if nodeID := os.Getenv("NODE_ID"); nodeID != "" {
		return nodeID
	}
	
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}
	
	return fmt.Sprintf("%s-%d", hostname, os.Getpid())
}

// GetNodeAddress returns the node address from environment or config
func GetNodeAddress() string {
	if addr := os.Getenv("NODE_ADDRESS"); addr != "" {
		return addr
	}
	
	port := 8081
	if portStr := os.Getenv("NODE_PORT"); portStr != "" {
		if p, err := strconv.Atoi(portStr); err == nil {
			port = p
		}
	}
	
	return fmt.Sprintf("localhost:%d", port)
} 