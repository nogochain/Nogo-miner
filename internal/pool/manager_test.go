package pool

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nogochain/nogo-miner/internal/config"
	"github.com/nogochain/nogo-miner/internal/logger"
)

func TestNewManager(t *testing.T) {
	log, _ := logger.New(logger.Config{Level: "info"})
	
	pools := []config.PoolConfig{
		{
			Name:     "pool1",
			URL:      "http://pool1:8080",
			Address:  "NOGO006f44f4319250563c65919062932cc1cd7bae04045c355bf53bcb9d7f785c0b473fabfd7c",
			Priority: 1,
			Enabled:  true,
		},
		{
			Name:     "pool2",
			URL:      "http://pool2:8080",
			Address:  "NOGO006f44f4319250563c65919062932cc1cd7bae04045c355bf53bcb9d7f785c0b473fabfd7c",
			Priority: 2,
			Enabled:  true,
		},
		{
			Name:     "pool3",
			URL:      "http://pool3:8080",
			Address:  "NOGO006f44f4319250563c65919062932cc1cd7bae04045c355bf53bcb9d7f785c0b473fabfd7c",
			Priority: 3,
			Enabled:  false,
		},
	}

	manager := NewManager(pools, log)
	if manager == nil {
		t.Fatal("Failed to create pool manager")
	}
	defer manager.Stop()

	if manager.GetPoolCount() != 2 {
		t.Errorf("Expected 2 pools, got %d", manager.GetPoolCount())
	}
}

func TestPoolSorting(t *testing.T) {
	log, _ := logger.New(logger.Config{Level: "info"})
	
	pools := []config.PoolConfig{
		{Name: "pool3", Priority: 3, Enabled: true, URL: "http://invalid", Address: "NOGO" + "00000000000000000000000000000000000000000000000000000000000000000000000000"},
		{Name: "pool1", Priority: 1, Enabled: true, URL: "http://invalid", Address: "NOGO" + "00000000000000000000000000000000000000000000000000000000000000000000000000"},
		{Name: "pool2", Priority: 2, Enabled: true, URL: "http://invalid", Address: "NOGO" + "00000000000000000000000000000000000000000000000000000000000000000000000000"},
	}

	manager := NewManager(pools, log)
	defer manager.Stop()

	// Verify sorting in stats (pools are sorted by priority)
	stats := manager.GetStats()
	if len(stats) != 3 {
		t.Errorf("Expected 3 stats, got %d", len(stats))
	}
	
	// Verify sorting - pools should be sorted by priority
	if stats[0].Priority != 1 || stats[0].Name != "pool1" {
		t.Errorf("Expected pool1 first (priority 1), got %s (priority %d)", stats[0].Name, stats[0].Priority)
	}
	if stats[1].Priority != 2 || stats[1].Name != "pool2" {
		t.Errorf("Expected pool2 second (priority 2), got %s (priority %d)", stats[1].Name, stats[1].Priority)
	}
	if stats[2].Priority != 3 || stats[2].Name != "pool3" {
		t.Errorf("Expected pool3 third (priority 3), got %s (priority %d)", stats[2].Name, stats[2].Priority)
	}
}

func TestManagerStartStop(t *testing.T) {
	// Create a mock HTTP server
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
	
	pools := []config.PoolConfig{
		{
			Name:     "test-pool",
			URL:      server.URL,
			Address:  "NOGO006f44f4319250563c65919062932cc1cd7bae04045c355bf53bcb9d7f785c0b473fabfd7c",
			Priority: 1,
			Enabled:  true,
		},
	}

	manager := NewManager(pools, log)
	
	ctx := context.Background()
	if err := manager.Start(ctx); err != nil {
		t.Fatalf("Failed to start manager: %v", err)
	}

	if !manager.IsConnected() {
		t.Error("Expected to be connected")
	}

	// Give health check goroutine time to start
	time.Sleep(100 * time.Millisecond)
	
	manager.Stop()

	// Wait a bit for stop to complete
	time.Sleep(100 * time.Millisecond)

	if manager.IsConnected() {
		t.Error("Expected to be disconnected after stop")
	}
}

func TestRecordShare(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"ok"}`))
		}
	}))
	defer server.Close()

	log, _ := logger.New(logger.Config{Level: "info"})
	
	pools := []config.PoolConfig{
		{
			Name:     "test-pool",
			URL:      server.URL,
			Address:  "NOGO006f44f4319250563c65919062932cc1cd7bae04045c355bf53bcb9d7f785c0b473fabfd7c",
			Priority: 1,
			Enabled:  true,
		},
	}

	manager := NewManager(pools, log)
	ctx := context.Background()
	manager.Start(ctx)
	defer manager.Stop()

	// Record some shares
	manager.RecordShare(true)
	manager.RecordShare(true)
	manager.RecordShare(false)
	manager.RecordShare(true)

	stats := manager.GetStats()
	if len(stats) != 1 {
		t.Fatalf("Expected 1 stat, got %d", len(stats))
	}

	if stats[0].Accepted != 3 {
		t.Errorf("Expected 3 accepted shares, got %d", stats[0].Accepted)
	}

	if stats[0].Rejected != 1 {
		t.Errorf("Expected 1 rejected share, got %d", stats[0].Rejected)
	}
}

func TestUpdateHashRate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"ok"}`))
		}
	}))
	defer server.Close()

	log, _ := logger.New(logger.Config{Level: "info"})
	
	pools := []config.PoolConfig{
		{
			Name:     "test-pool",
			URL:      server.URL,
			Address:  "NOGO006f44f4319250563c65919062932cc1cd7bae04045c355bf53bcb9d7f785c0b473fabfd7c",
			Priority: 1,
			Enabled:  true,
		},
	}

	manager := NewManager(pools, log)
	ctx := context.Background()
	manager.Start(ctx)
	defer manager.Stop()

	// Update hash rate
	manager.UpdateHashRate(1234)

	stats := manager.GetStats()
	if stats[0].HashRate != 1234 {
		t.Errorf("Expected hash rate 1234, got %d", stats[0].HashRate)
	}
}

func TestGetAddress(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"ok"}`))
		}
	}))
	defer server.Close()

	log, _ := logger.New(logger.Config{Level: "info"})
	
	expectedAddress := "NOGO006f44f4319250563c65919062932cc1cd7bae04045c355bf53bcb9d7f785c0b473fabfd7c"
	pools := []config.PoolConfig{
		{
			Name:     "test-pool",
			URL:      server.URL,
			Address:  expectedAddress,
			Priority: 1,
			Enabled:  true,
		},
	}

	manager := NewManager(pools, log)
	ctx := context.Background()
	manager.Start(ctx)
	defer manager.Stop()

	address := manager.GetAddress()
	if address != expectedAddress {
		t.Errorf("Expected address %s, got %s", expectedAddress, address)
	}
}

func TestHealthCheckLoop(t *testing.T) {
	healthCheckCount := 0
	
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			healthCheckCount++
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"ok"}`))
		}
	}))
	defer server.Close()

	log, _ := logger.New(logger.Config{Level: "info"})
	
	pools := []config.PoolConfig{
		{
			Name:     "test-pool",
			URL:      server.URL,
			Address:  "NOGO006f44f4319250563c65919062932cc1cd7bae04045c355bf53bcb9d7f785c0b473fabfd7c",
			Priority: 1,
			Enabled:  true,
		},
	}

	manager := NewManager(pools, log)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	
	manager.Start(ctx)
	defer manager.Stop()

	// Wait for health checks
	<-time.After(2500 * time.Millisecond)

	// Should have at least 1 health check
	if healthCheckCount < 1 {
		t.Errorf("Expected at least 1 health check, got %d", healthCheckCount)
	}
}

func TestPoolStats(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"ok"}`))
		}
	}))
	defer server.Close()

	log, _ := logger.New(logger.Config{Level: "info"})
	
	pools := []config.PoolConfig{
		{
			Name:     "test-pool",
			URL:      server.URL,
			Address:  "NOGO006f44f4319250563c65919062932cc1cd7bae04045c355bf53bcb9d7f785c0b473fabfd7c",
			Priority: 1,
			Enabled:  true,
		},
	}

	manager := NewManager(pools, log)
	ctx := context.Background()
	manager.Start(ctx)
	defer manager.Stop()

	stats := manager.GetStats()
	if len(stats) != 1 {
		t.Fatalf("Expected 1 stat, got %d", len(stats))
	}

	if stats[0].Name != "test-pool" {
		t.Errorf("Expected pool name 'test-pool', got %s", stats[0].Name)
	}

	if !stats[0].Connected {
		t.Error("Expected pool to be connected")
	}

	if stats[0].Priority != 1 {
		t.Errorf("Expected priority 1, got %d", stats[0].Priority)
	}
}
