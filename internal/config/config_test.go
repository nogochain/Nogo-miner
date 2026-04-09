package config

import (
	"os"
	"testing"
)

func TestLoad(t *testing.T) {
	// Create temporary config file
	tmpfile, err := os.CreateTemp("", "config-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	configData := `{
		"rpc": {
			"url": "http://localhost:8080",
			"ws_url": "ws://localhost:8080/ws",
			"timeout_seconds": 30
		},
		"pools": [{
			"name": "test-pool",
			"url": "http://localhost:8080",
			"address": "NOGO006f44f4319250563c65919062932cc1cd7bae04045c355bf53bcb9d7f785c0b473fabfd7c",
			"priority": 1,
			"enabled": true
		}],
		"miner": {
			"threads": 2,
			"batch_size": 500
		},
		"logging": {
			"level": "info",
			"file": "test.log"
		},
		"monitor": {
			"enabled": true,
			"update_interval_seconds": 10
		}
	}`

	if _, err := tmpfile.Write([]byte(configData)); err != nil {
		t.Fatal(err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatal(err)
	}

	// Load configuration
	cfg, err := Load(tmpfile.Name())
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify configuration
	if cfg.RPC.URL != "http://localhost:8080" {
		t.Errorf("Expected RPC URL http://localhost:8080, got %s", cfg.RPC.URL)
	}
	if cfg.Miner.Threads != 2 {
		t.Errorf("Expected 2 threads, got %d", cfg.Miner.Threads)
	}
	if len(cfg.Pools) != 1 {
		t.Errorf("Expected 1 pool, got %d", len(cfg.Pools))
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: &Config{
				RPC: RPCConfig{
					URL:               "http://localhost:8080",
					TimeoutSeconds:    30,
					MaxRetries:        5,
					RetryDelaySeconds: 2,
				},
				Pools: []PoolConfig{
					{
						Name:     "test",
						URL:      "http://localhost:8080",
						Address:  "NOGO006f44f4319250563c65919062932cc1cd7bae04045c355bf53bcb9d7f785c0b473fabfd7c",
						Priority: 1,
						Enabled:  true,
					},
				},
				Miner: MinerConfig{
					Threads:         2,
					BatchSize:       500,
					ShareDifficulty: 1000,
				},
				Logging: LoggingConfig{
					Level:      "info",
					File:       "test.log",
					MaxSizeMB:  10,
					MaxBackups: 3,
				},
				Monitor: MonitorConfig{
					Enabled:             true,
					UpdateIntervalSeconds: 10,
				},
			},
			wantErr: false,
		},
		{
			name: "missing RPC URL",
			config: &Config{
				RPC: RPCConfig{
					TimeoutSeconds: 30,
				},
				Pools: []PoolConfig{
					{
						Name:     "test",
						URL:      "http://localhost:8080",
						Address:  "NOGO006f44f4319250563c65919062932cc1cd7bae04045c355bf53bcb9d7f785c0b473fabfd7c",
						Priority: 1,
					},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid address",
			config: &Config{
				RPC: RPCConfig{
					URL:            "http://localhost:8080",
					TimeoutSeconds: 30,
				},
				Pools: []PoolConfig{
					{
						Name:     "test",
						URL:      "http://localhost:8080",
						Address:  "INVALID",
						Priority: 1,
					},
				},
			},
			wantErr: true,
		},
		{
			name: "no pools",
			config: &Config{
				RPC: RPCConfig{
					URL:            "http://localhost:8080",
					TimeoutSeconds: 30,
				},
				Pools: []PoolConfig{},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestIsValidAddress(t *testing.T) {
	tests := []struct {
		address string
		valid   bool
	}{
		{"NOGO006f44f4319250563c65919062932cc1cd7bae04045c355bf53bcb9d7f785c0b473fabfd7c", true},
		{"NOGO" + "00000000000000000000000000000000000000000000000000000000000000000000000000", true},
		{"NOGO0000000000000000000000000000000000000000000000000000000000000000000000000", false}, // too short
		{"NOGO000000000000000000000000000000000000000000000000000000000000000000000000000000", false}, // too long
		{"NOGX00000000000000000000000000000000000000000000000000000000000000000000000000", false}, // wrong prefix
		{"NOGO0000000000000000000000000000000000000000000000000000000000000000000000000g", false}, // invalid hex
	}

	for _, tt := range tests {
		t.Run(tt.address, func(t *testing.T) {
			if got := isValidAddress(tt.address); got != tt.valid {
				t.Errorf("isValidAddress(%s) = %v, want %v", tt.address, got, tt.valid)
			}
		})
	}
}

func TestGetEnabledPools(t *testing.T) {
	cfg := &Config{
		Pools: []PoolConfig{
			{Name: "pool1", URL: "http://pool1", Address: "NOGO" + "00000000000000000000000000000000000000000000000000000000000000000000000000", Priority: 3, Enabled: true},
			{Name: "pool2", URL: "http://pool2", Address: "NOGO" + "00000000000000000000000000000000000000000000000000000000000000000000000000", Priority: 1, Enabled: true},
			{Name: "pool3", URL: "http://pool3", Address: "NOGO" + "00000000000000000000000000000000000000000000000000000000000000000000000000", Priority: 2, Enabled: false},
		},
	}

	enabled := cfg.GetEnabledPools()
	
	if len(enabled) != 2 {
		t.Errorf("Expected 2 enabled pools, got %d", len(enabled))
	}
	
	if enabled[0].Priority != 1 || enabled[0].Name != "pool2" {
		t.Errorf("Expected pool2 first (priority 1), got %s (priority %d)", enabled[0].Name, enabled[0].Priority)
	}
	
	if enabled[1].Priority != 3 || enabled[1].Name != "pool1" {
		t.Errorf("Expected pool1 second (priority 3), got %s (priority %d)", enabled[1].Name, enabled[1].Priority)
	}
}

func TestOverrideFromEnv(t *testing.T) {
	cfg := &Config{
		RPC: RPCConfig{
			URL: "http://original:8080",
		},
		Pools: []PoolConfig{
			{Address: "NOGO" + "00000000000000000000000000000000000000000000000000000000000000000000000000"},
		},
		Miner: MinerConfig{
			Threads: 2,
		},
		Logging: LoggingConfig{
			Level: "info",
		},
	}

	// Set environment variables
	os.Setenv("NOGOMINER_RPC_URL", "http://env:8080")
	os.Setenv("NOGOMINER_THREADS", "4")
	os.Setenv("NOGOMINER_ADDRESS", "NOGO" + "11111111111111111111111111111111111111111111111111111111111111111111111111")
	os.Setenv("NOGOMINER_LOG_LEVEL", "debug")
	defer func() {
		os.Unsetenv("NOGOMINER_RPC_URL")
		os.Unsetenv("NOGOMINER_THREADS")
		os.Unsetenv("NOGOMINER_ADDRESS")
		os.Unsetenv("NOGOMINER_LOG_LEVEL")
	}()

	cfg.overrideFromEnv()

	if cfg.RPC.URL != "http://env:8080" {
		t.Errorf("Expected RPC URL http://env:8080, got %s", cfg.RPC.URL)
	}
	if cfg.Miner.Threads != 4 {
		t.Errorf("Expected 4 threads, got %d", cfg.Miner.Threads)
	}
	if cfg.Pools[0].Address != "NOGO" + "11111111111111111111111111111111111111111111111111111111111111111111111111" {
		t.Errorf("Pool address not overridden")
	}
	if cfg.Logging.Level != "debug" {
		t.Errorf("Expected log level debug, got %s", cfg.Logging.Level)
	}
}
