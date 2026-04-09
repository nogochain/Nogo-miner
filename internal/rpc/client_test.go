package rpc

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nogochain/nogo-miner/internal/config"
	"github.com/nogochain/nogo-miner/internal/logger"
)

func TestNewClient(t *testing.T) {
	log, _ := logger.New(logger.Config{Level: "info"})
	
	cfg := &config.RPCConfig{
		URL:               "http://localhost:8080",
		WSURL:             "ws://localhost:8080/ws",
		TimeoutSeconds:    30,
		MaxRetries:        3,
		RetryDelaySeconds: 1,
	}

	client := NewClient(cfg, log)
	if client == nil {
		t.Fatal("Failed to create client")
	}
	defer client.Close()

	if client.baseURL != cfg.URL {
		t.Errorf("Expected baseURL %s, got %s", cfg.URL, client.baseURL)
	}
	if client.wsURL != cfg.WSURL {
		t.Errorf("Expected wsURL %s, got %s", cfg.WSURL, client.wsURL)
	}
	if client.timeout != time.Duration(cfg.TimeoutSeconds)*time.Second {
		t.Errorf("Expected timeout %v, got %v", time.Duration(cfg.TimeoutSeconds)*time.Second, client.timeout)
	}
}

func TestHealthCheck(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"ok"}`))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	log, _ := logger.New(logger.Config{Level: "info"})
	cfg := &config.RPCConfig{
		URL:               server.URL,
		TimeoutSeconds:    5,
		MaxRetries:        1,
		RetryDelaySeconds: 1,
	}

	client := NewClient(cfg, log)
	defer client.Close()

	ctx := context.Background()
	err := client.HealthCheck(ctx)
	if err != nil {
		t.Fatalf("HealthCheck failed: %v", err)
	}

	if !client.IsConnected() {
		t.Error("Expected client to be connected")
	}
}

func TestGetNodeInfo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/chain/info" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{
				"version": "1.0.0",
				"chainId": 1,
				"height": 12345,
				"peersCount": 10,
				"syncing": false,
				"networkId": "mainnet",
				"protocolVersion": 1
			}`))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	log, _ := logger.New(logger.Config{Level: "info"})
	cfg := &config.RPCConfig{
		URL:               server.URL,
		TimeoutSeconds:    5,
		MaxRetries:        1,
		RetryDelaySeconds: 1,
	}

	client := NewClient(cfg, log)
	defer client.Close()

	ctx := context.Background()
	info, err := client.GetNodeInfo(ctx)
	if err != nil {
		t.Fatalf("GetNodeInfo failed: %v", err)
	}

	if info.Version != "1.0.0" {
		t.Errorf("Expected version 1.0.0, got %s", info.Version)
	}
	if info.ChainID != 1 {
		t.Errorf("Expected chainId 1, got %d", info.ChainID)
	}
	if info.Height != 12345 {
		t.Errorf("Expected height 12345, got %d", info.Height)
	}
	if info.PeersCount != 10 {
		t.Errorf("Expected 10 peers, got %d", info.PeersCount)
	}
}

func TestGetBlockTemplate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/block/template" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{
				"height": 100,
				"prevHash": "0000000000000000000000000000000000000000000000000000000000000000",
				"merkleRoot": "1111111111111111111111111111111111111111111111111111111111111111",
				"timestamp": 1234567890,
				"difficultyBits": 18,
				"minerAddress": "NOGO006f44f4319250563c65919062932cc1cd7bae04045c355bf53bcb9d7f785c0b473fabfd7c",
				"transactions": [],
				"coinbaseTx": null,
				"target": "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
				"chainId": 1
			}`))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	log, _ := logger.New(logger.Config{Level: "info"})
	cfg := &config.RPCConfig{
		URL:               server.URL,
		TimeoutSeconds:    5,
		MaxRetries:        1,
		RetryDelaySeconds: 1,
	}

	client := NewClient(cfg, log)
	defer client.Close()

	ctx := context.Background()
	template, err := client.GetBlockTemplate(ctx)
	if err != nil {
		t.Fatalf("GetBlockTemplate failed: %v", err)
	}

	if template.Height != 100 {
		t.Errorf("Expected height 100, got %d", template.Height)
	}
	if template.DifficultyBits != 18 {
		t.Errorf("Expected difficultyBits 18, got %d", template.DifficultyBits)
	}
	if template.ChainID != 1 {
		t.Errorf("Expected chainId 1, got %d", template.ChainID)
	}
}

func TestSubmitWork(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/mining/submit" && r.Method == http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{
				"accepted": true,
				"message": "Block submitted successfully"
			}`))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	log, _ := logger.New(logger.Config{Level: "info"})
	cfg := &config.RPCConfig{
		URL:               server.URL,
		TimeoutSeconds:    5,
		MaxRetries:        1,
		RetryDelaySeconds: 1,
	}

	client := NewClient(cfg, log)
	defer client.Close()

	ctx := context.Background()
	req := SubmitWorkRequest{
		Height:     100,
		Nonce:      12345,
		BlockHash:  "abc123",
		PrevHash:   "def456",
		MerkleRoot: "ghi789",
		Timestamp:  1234567890,
	}

	resp, err := client.SubmitWork(ctx, req)
	if err != nil {
		t.Fatalf("SubmitWork failed: %v", err)
	}

	if !resp.Accepted {
		t.Error("Expected work to be accepted")
	}
	if resp.Message != "Block submitted successfully" {
		t.Errorf("Expected message 'Block submitted successfully', got %s", resp.Message)
	}
}

func TestRetry(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 2 {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"ok"}`))
		}
	}))
	defer server.Close()

	log, _ := logger.New(logger.Config{Level: "debug"})
	cfg := &config.RPCConfig{
		URL:               server.URL,
		TimeoutSeconds:    5,
		MaxRetries:        3,
		RetryDelaySeconds: 1,
	}

	client := NewClient(cfg, log)
	defer client.Close()

	ctx := context.Background()
	err := client.HealthCheck(ctx)
	if err != nil {
		t.Fatalf("HealthCheck failed after retries: %v", err)
	}

	if attempts != 2 {
		t.Errorf("Expected 2 attempts, got %d", attempts)
	}
}

func TestConnectionStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	log, _ := logger.New(logger.Config{Level: "info"})
	cfg := &config.RPCConfig{
		URL:               server.URL,
		TimeoutSeconds:    5,
		MaxRetries:        1,
		RetryDelaySeconds: 1,
	}

	client := NewClient(cfg, log)
	defer client.Close()

	if client.IsConnected() {
		t.Error("Expected client to be disconnected initially")
	}

	ctx := context.Background()
	err := client.HealthCheck(ctx)
	if err != nil {
		t.Fatalf("HealthCheck failed: %v", err)
	}

	if !client.IsConnected() {
		t.Error("Expected client to be connected after successful request")
	}

	client.Close()

	if client.IsConnected() {
		t.Error("Expected client to be disconnected after Close")
	}
}

func TestBytesToHex(t *testing.T) {
	tests := []struct {
		input  []byte
		output string
	}{
		{[]byte{0x00, 0x01, 0x02, 0x03}, "00010203"},
		{[]byte{}, ""},
		{[]byte{0xff}, "ff"},
	}

	for _, tt := range tests {
		result := BytesToHex(tt.input)
		if result != tt.output {
			t.Errorf("BytesToHex(%v) = %s, want %s", tt.input, result, tt.output)
		}
	}
}

func TestHexToBytes(t *testing.T) {
	tests := []struct {
		input  string
		output []byte
		hasErr bool
	}{
		{"00010203", []byte{0x00, 0x01, 0x02, 0x03}, false},
		{"", []byte{}, false},
		{"ff", []byte{0xff}, false},
		{"invalid", nil, true},
	}

	for _, tt := range tests {
		result, err := HexToBytes(tt.input)
		if (err != nil) != tt.hasErr {
			t.Errorf("HexToBytes(%s) error = %v, wantErr %v", tt.input, err, tt.hasErr)
		}
		if !tt.hasErr && string(result) != string(tt.output) {
			t.Errorf("HexToBytes(%s) = %v, want %v", tt.input, result, tt.output)
		}
	}
}
