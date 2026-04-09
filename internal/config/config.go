// Package config provides configuration management for NogoMiner
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config represents the complete miner configuration
type Config struct {
	RPC      RPCConfig      `json:"rpc"`
	Pools    []PoolConfig   `json:"pools"`
	Miner    MinerConfig    `json:"miner"`
	Logging  LoggingConfig  `json:"logging"`
	Monitor  MonitorConfig  `json:"monitor"`
}

// RPCConfig represents RPC connection configuration
type RPCConfig struct {
	URL              string `json:"url"`
	WSURL            string `json:"ws_url"`
	TimeoutSeconds   int    `json:"timeout_seconds"`
	MaxRetries       int    `json:"max_retries"`
	RetryDelaySeconds int   `json:"retry_delay_seconds"`
}

// PoolConfig represents mining pool configuration
type PoolConfig struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	WSURL    string `json:"ws_url"`
	Address  string `json:"address"`
	Priority int    `json:"priority"`
	Enabled  bool   `json:"enabled"`
}

// MinerConfig represents miner configuration
type MinerConfig struct {
	Threads        int `json:"threads"`
	BatchSize      int `json:"batch_size"`
	ShareDifficulty int `json:"share_difficulty"`
}

// LoggingConfig represents logging configuration
type LoggingConfig struct {
	Level       string `json:"level"`
	File        string `json:"file"`
	MaxSizeMB   int    `json:"max_size_mb"`
	MaxBackups  int    `json:"max_backups"`
	MaxAgeDays  int    `json:"max_age_days"`
	Compress    bool   `json:"compress"`
	JSONFormat  bool   `json:"json_format"`
}

// MonitorConfig represents monitoring configuration
type MonitorConfig struct {
	Enabled            bool `json:"enabled"`
	UpdateIntervalSeconds int `json:"update_interval_seconds"`
	PrometheusEnabled  bool `json:"prometheus_enabled"`
	PrometheusPort     int  `json:"prometheus_port"`
}

// Load loads configuration from file
func Load(configPath string) (*Config, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config file: %w", err)
	}

	// Apply defaults
	cfg.applyDefaults()

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	// Override with environment variables
	cfg.overrideFromEnv()

	return &cfg, nil
}

// applyDefaults applies default values to configuration
func (c *Config) applyDefaults() {
	// RPC defaults
	if c.RPC.TimeoutSeconds == 0 {
		c.RPC.TimeoutSeconds = 30
	}
	if c.RPC.MaxRetries == 0 {
		c.RPC.MaxRetries = 5
	}
	if c.RPC.RetryDelaySeconds == 0 {
		c.RPC.RetryDelaySeconds = 2
	}

	// Miner defaults
	if c.Miner.Threads == 0 {
		// Auto-detect CPU cores
		c.Miner.Threads = 1
	}
	if c.Miner.BatchSize == 0 {
		c.Miner.BatchSize = 1000
	}
	if c.Miner.ShareDifficulty == 0 {
		c.Miner.ShareDifficulty = 1000
	}

	// Logging defaults
	if c.Logging.Level == "" {
		c.Logging.Level = "info"
	}
	if c.Logging.File == "" {
		c.Logging.File = "nogominer.log"
	}
	if c.Logging.MaxSizeMB == 0 {
		c.Logging.MaxSizeMB = 10
	}
	if c.Logging.MaxBackups == 0 {
		c.Logging.MaxBackups = 3
	}
	if c.Logging.MaxAgeDays == 0 {
		c.Logging.MaxAgeDays = 30
	}

	// Monitor defaults
	if !c.Monitor.Enabled {
		c.Monitor.Enabled = true
	}
	if c.Monitor.UpdateIntervalSeconds == 0 {
		c.Monitor.UpdateIntervalSeconds = 10
	}
}

// Validate validates the configuration
func (c *Config) Validate() error {
	// Validate RPC config
	if c.RPC.URL == "" {
		return fmt.Errorf("rpc.url is required")
	}
	if c.RPC.TimeoutSeconds < 1 {
		return fmt.Errorf("rpc.timeout_seconds must be >= 1")
	}
	if c.RPC.MaxRetries < 0 {
		return fmt.Errorf("rpc.max_retries must be >= 0")
	}
	if c.RPC.RetryDelaySeconds < 1 {
		return fmt.Errorf("rpc.retry_delay_seconds must be >= 1")
	}

	// Validate pools
	if len(c.Pools) == 0 {
		return fmt.Errorf("at least one pool is required")
	}
	for i, pool := range c.Pools {
		if pool.Name == "" {
			return fmt.Errorf("pools[%d].name is required", i)
		}
		if pool.URL == "" {
			return fmt.Errorf("pools[%d].url is required", i)
		}
		if pool.Address == "" {
			return fmt.Errorf("pools[%d].address is required", i)
		}
		if !isValidAddress(pool.Address) {
			return fmt.Errorf("pools[%d].address is invalid: %s", i, pool.Address)
		}
		if pool.Priority < 1 {
			return fmt.Errorf("pools[%d].priority must be >= 1", i)
		}
	}

	// Validate miner config
	if c.Miner.Threads < 0 {
		return fmt.Errorf("miner.threads must be >= 0")
	}
	if c.Miner.BatchSize < 1 {
		return fmt.Errorf("miner.batch_size must be >= 1")
	}
	if c.Miner.ShareDifficulty < 1 {
		return fmt.Errorf("miner.share_difficulty must be >= 1")
	}

	// Validate logging config
	validLevels := map[string]bool{
		"debug": true,
		"info":  true,
		"warn":  true,
		"error": true,
	}
	if !validLevels[c.Logging.Level] {
		return fmt.Errorf("logging.level must be one of: debug, info, warn, error")
	}
	if c.Logging.MaxSizeMB < 1 {
		return fmt.Errorf("logging.max_size_mb must be >= 1")
	}
	if c.Logging.MaxBackups < 0 {
		return fmt.Errorf("logging.max_backups must be >= 0")
	}
	if c.Logging.MaxAgeDays < 0 {
		return fmt.Errorf("logging.max_age_days must be >= 0")
	}

	// Validate monitor config
	if c.Monitor.UpdateIntervalSeconds < 1 {
		return fmt.Errorf("monitor.update_interval_seconds must be >= 1")
	}
	if c.Monitor.PrometheusEnabled && (c.Monitor.PrometheusPort < 1 || c.Monitor.PrometheusPort > 65535) {
		return fmt.Errorf("monitor.prometheus_port must be between 1 and 65535")
	}

	return nil
}

// overrideFromEnv overrides configuration with environment variables
func (c *Config) overrideFromEnv() {
	if v := os.Getenv("NOGOMINER_RPC_URL"); v != "" {
		c.RPC.URL = v
	}
	if v := os.Getenv("NOGOMINER_WS_URL"); v != "" {
		c.RPC.WSURL = v
	}
	if v := os.Getenv("NOGOMINER_THREADS"); v != "" {
		if threads, err := strconv.Atoi(v); err == nil {
			c.Miner.Threads = threads
		}
	}
	if v := os.Getenv("NOGOMINER_ADDRESS"); v != "" {
		// Override first pool address
		if len(c.Pools) > 0 {
			c.Pools[0].Address = v
		}
	}
	if v := os.Getenv("NOGOMINER_LOG_LEVEL"); v != "" {
		c.Logging.Level = v
	}
}

// isValidAddress validates NogoChain address format
func isValidAddress(address string) bool {
	// Address format: NOGO + version byte (1 byte) + hash (32 bytes) + checksum (4 bytes)
	// Total: 4 + 2 + 64 + 8 = 78 characters
	if len(address) != 78 {
		return false
	}
	if address[:4] != "NOGO" {
		return false
	}
	// Check if remaining characters are valid hex
	for i := 4; i < len(address); i++ {
		c := address[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

// GetTimeout returns RPC timeout as time.Duration
func (c *RPCConfig) GetTimeout() time.Duration {
	return time.Duration(c.TimeoutSeconds) * time.Second
}

// GetRetryDelay returns retry delay as time.Duration
func (c *RPCConfig) GetRetryDelay() time.Duration {
	return time.Duration(c.RetryDelaySeconds) * time.Second
}

// GetUpdateInterval returns monitor update interval as time.Duration
func (c *MonitorConfig) GetUpdateInterval() time.Duration {
	return time.Duration(c.UpdateIntervalSeconds) * time.Second
}

// GetEnabledPools returns enabled pools sorted by priority
func (c *Config) GetEnabledPools() []PoolConfig {
	enabled := make([]PoolConfig, 0)
	for _, pool := range c.Pools {
		if pool.Enabled {
			enabled = append(enabled, pool)
		}
	}
	
	// Sort by priority (lower number = higher priority)
	for i := 0; i < len(enabled); i++ {
		for j := i + 1; j < len(enabled); j++ {
			if enabled[j].Priority < enabled[i].Priority {
				enabled[i], enabled[j] = enabled[j], enabled[i]
			}
		}
	}
	
	return enabled
}
