package monitor

import (
	"context"
	"testing"
	"time"

	"github.com/nogochain/nogo-miner/internal/config"
	"github.com/nogochain/nogo-miner/internal/logger"
)

func TestNewMonitor(t *testing.T) {
	log, _ := logger.New(logger.Config{Level: "info"})
	
	cfg := config.MonitorConfig{
		Enabled:             true,
		UpdateIntervalSeconds: 10,
	}

	monitor := NewMonitor(cfg, log)
	if monitor == nil {
		t.Fatal("Failed to create monitor")
	}

	stats := monitor.GetStats()
	if stats == nil {
		t.Error("Expected stats to be initialized")
	}

	if stats.StartTime.IsZero() {
		t.Error("Expected start time to be set")
	}
}

func TestMonitorStartStop(t *testing.T) {
	log, _ := logger.New(logger.Config{Level: "info"})
	
	cfg := config.MonitorConfig{
		Enabled:             true,
		UpdateIntervalSeconds: 1,
	}

	monitor := NewMonitor(cfg, log)
	ctx := context.Background()
	
	monitor.Start(ctx)
	
	// Let it run for a bit
	time.Sleep(100 * time.Millisecond)
	
	monitor.Stop()
}

func TestUpdateHashRate(t *testing.T) {
	log, _ := logger.New(logger.Config{Level: "info"})
	cfg := config.MonitorConfig{Enabled: true}
	monitor := NewMonitor(cfg, log)

	monitor.UpdateHashRate(1234)
	
	stats := monitor.GetStats()
	if stats.HashRate != 1234 {
		t.Errorf("Expected hash rate 1234, got %d", stats.HashRate)
	}
}

func TestAddHashes(t *testing.T) {
	log, _ := logger.New(logger.Config{Level: "info"})
	cfg := config.MonitorConfig{Enabled: true}
	monitor := NewMonitor(cfg, log)

	monitor.AddHashes(100)
	monitor.AddHashes(200)
	
	stats := monitor.GetStats()
	if stats.TotalHashes != 300 {
		t.Errorf("Expected total hashes 300, got %d", stats.TotalHashes)
	}
}

func TestRecordShare(t *testing.T) {
	log, _ := logger.New(logger.Config{Level: "info"})
	cfg := config.MonitorConfig{Enabled: true}
	monitor := NewMonitor(cfg, log)

	// Record some shares
	monitor.RecordShare(true, "00000001")
	monitor.RecordShare(true, "00000002")
	monitor.RecordShare(false, "00000003")
	monitor.RecordShare(true, "00000000")
	
	stats := monitor.GetStats()
	
	if stats.AcceptedShares != 3 {
		t.Errorf("Expected 3 accepted shares, got %d", stats.AcceptedShares)
	}
	
	if stats.RejectedShares != 1 {
		t.Errorf("Expected 1 rejected share, got %d", stats.RejectedShares)
	}
	
	// Best share should be the lowest difficulty
	if stats.BestShare != "00000000" {
		t.Errorf("Expected best share 00000000, got %s", stats.BestShare)
	}
}

func TestUpdatePoolInfo(t *testing.T) {
	log, _ := logger.New(logger.Config{Level: "info"})
	cfg := config.MonitorConfig{Enabled: true}
	monitor := NewMonitor(cfg, log)

	monitor.UpdatePoolInfo("test-pool", "http://pool:8080")
	
	stats := monitor.GetStats()
	if stats.PoolName != "test-pool" {
		t.Errorf("Expected pool name 'test-pool', got %s", stats.PoolName)
	}
	if stats.PoolURL != "http://pool:8080" {
		t.Errorf("Expected pool URL 'http://pool:8080', got %s", stats.PoolURL)
	}
}

func TestUpdateWorkers(t *testing.T) {
	log, _ := logger.New(logger.Config{Level: "info"})
	cfg := config.MonitorConfig{Enabled: true}
	monitor := NewMonitor(cfg, log)

	monitor.UpdateWorkers(4)
	
	stats := monitor.GetStats()
	if stats.ActiveWorkers != 4 {
		t.Errorf("Expected 4 active workers, got %d", stats.ActiveWorkers)
	}
}

func TestUpdateHardware(t *testing.T) {
	log, _ := logger.New(logger.Config{Level: "info"})
	cfg := config.MonitorConfig{Enabled: true}
	monitor := NewMonitor(cfg, log)

	monitor.UpdateHardware(65.5, 1200, 150.0)
	
	stats := monitor.GetStats()
	if stats.Temperature != 65.5 {
		t.Errorf("Expected temperature 65.5, got %f", stats.Temperature)
	}
	if stats.FanSpeed != 1200 {
		t.Errorf("Expected fan speed 1200, got %d", stats.FanSpeed)
	}
	if stats.PowerConsumption != 150.0 {
		t.Errorf("Expected power consumption 150.0, got %f", stats.PowerConsumption)
	}
}

func TestAddReward(t *testing.T) {
	log, _ := logger.New(logger.Config{Level: "info"})
	cfg := config.MonitorConfig{Enabled: true}
	monitor := NewMonitor(cfg, log)

	monitor.AddReward(0.5)
	monitor.AddReward(0.25)
	
	stats := monitor.GetStats()
	if stats.TotalReward != 0.75 {
		t.Errorf("Expected total reward 0.75, got %f", stats.TotalReward)
	}
}

func TestGetEfficiency(t *testing.T) {
	log, _ := logger.New(logger.Config{Level: "info"})
	cfg := config.MonitorConfig{Enabled: true}
	monitor := NewMonitor(cfg, log)

	// No shares yet
	efficiency := monitor.GetEfficiency()
	if efficiency != 100.0 {
		t.Errorf("Expected efficiency 100.0, got %f", efficiency)
	}

	// Record some shares
	monitor.RecordShare(true, "")
	monitor.RecordShare(true, "")
	monitor.RecordShare(true, "")
	monitor.RecordShare(false, "")
	
	efficiency = monitor.GetEfficiency()
	expectedEfficiency := 75.0 // 3/4 = 75%
	
	if efficiency != expectedEfficiency {
		t.Errorf("Expected efficiency %.1f, got %f", expectedEfficiency, efficiency)
	}
}

func TestGetSummary(t *testing.T) {
	log, _ := logger.New(logger.Config{Level: "info"})
	cfg := config.MonitorConfig{Enabled: true}
	monitor := NewMonitor(cfg, log)

	// Add some data
	monitor.UpdateHashRate(1234567)
	monitor.RecordShare(true, "")
	monitor.RecordShare(true, "")
	monitor.RecordShare(false, "")
	
	summary := monitor.GetSummary()
	
	if summary == "" {
		t.Error("Expected non-empty summary")
	}
	
	t.Logf("Summary: %s", summary)
}

func TestReset(t *testing.T) {
	log, _ := logger.New(logger.Config{Level: "info"})
	cfg := config.MonitorConfig{Enabled: true}
	monitor := NewMonitor(cfg, log)

	// Add some data
	monitor.UpdateHashRate(1000)
	monitor.AddHashes(500)
	monitor.RecordShare(true, "")
	
	// Reset
	monitor.Reset()
	
	stats := monitor.GetStats()
	if stats.HashRate != 0 {
		t.Errorf("Expected hash rate 0 after reset, got %d", stats.HashRate)
	}
	if stats.TotalHashes != 0 {
		t.Errorf("Expected total hashes 0 after reset, got %d", stats.TotalHashes)
	}
	if stats.AcceptedShares != 0 {
		t.Errorf("Expected accepted shares 0 after reset, got %d", stats.AcceptedShares)
	}
}

func TestTriggerUpdate(t *testing.T) {
	log, _ := logger.New(logger.Config{Level: "info"})
	cfg := config.MonitorConfig{
		Enabled:             true,
		UpdateIntervalSeconds: 60, // Long interval
	}
	monitor := NewMonitor(cfg, log)
	ctx := context.Background()
	
	monitor.Start(ctx)
	defer monitor.Stop()

	// Trigger manual update
	monitor.TriggerUpdate()
	
	// Give it time to process
	time.Sleep(100 * time.Millisecond)
	
	// Should not panic or block
}

func TestFormatHashRate(t *testing.T) {
	tests := []struct {
		input  uint64
		output string
	}{
		{100, "100 H/s"},
		{1000, "1.00 KH/s"},
		{1500, "1.50 KH/s"},
		{1000000, "1.00 MH/s"},
		{1500000, "1.50 MH/s"},
		{1000000000, "1.00 GH/s"},
		{1500000000, "1.50 GH/s"},
	}

	for _, tt := range tests {
		result := formatHashRate(tt.input)
		if result != tt.output {
			t.Errorf("formatHashRate(%d) = %s, want %s", tt.input, result, tt.output)
		}
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		input  time.Duration
		output string
	}{
		{5 * time.Second, "5s"},
		{65 * time.Second, "1m 5s"},
		{3665 * time.Second, "1h 1m 5s"},
	}

	for _, tt := range tests {
		result := formatDuration(tt.input)
		if result != tt.output {
			t.Errorf("formatDuration(%v) = %s, want %s", tt.input, result, tt.output)
		}
	}
}

func TestCallbackRegistration(t *testing.T) {
	log, _ := logger.New(logger.Config{Level: "info"})
	cfg := config.MonitorConfig{Enabled: true}
	monitor := NewMonitor(cfg, log)

	callbackCalled := false
	monitor.RegisterCallback(func(stats *Stats) {
		callbackCalled = true
	})

	// Note: callbacks are not automatically called in this implementation
	// They would be called when statistics are updated
	if callbackCalled {
		t.Error("Callback should not be called immediately")
	}
}

func TestSetDisplayFunc(t *testing.T) {
	log, _ := logger.New(logger.Config{Level: "info"})
	cfg := config.MonitorConfig{Enabled: true}
	monitor := NewMonitor(cfg, log)

	// Set display function (just verify it doesn't panic)
	monitor.SetDisplayFunc(func(stats *Stats) {
		// Display function implementation
		_ = stats
	})

	// Note: display function is called in displayLoop
	// For this test, we just verify it's set without error
}
