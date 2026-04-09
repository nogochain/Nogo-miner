package miner

import (
	"context"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nogochain/nogo-miner/internal/config"
	"github.com/nogochain/nogo-miner/internal/logger"
	"github.com/nogochain/nogo-miner/internal/monitor"
	"github.com/nogochain/nogo-miner/internal/pool"
	"github.com/nogochain/nogo-miner/pkg/nogopow"
)

func TestNewMiner(t *testing.T) {
	log, _ := logger.New(logger.Config{Level: "info"})
	
	cfg := &MinerConfig{
		Threads:        2,
		BatchSize:      1000,
		ShareDifficulty: 1000,
	}

	// Create pool manager
	pools := []config.PoolConfig{
		{
			Name:     "test-pool",
			URL:      "http://invalid",
			Address:  "NOGO006f44f4319250563c65919062932cc1cd7bae04045c355bf53bcb9d7f785c0b473fabfd7c",
			Priority: 1,
			Enabled:  true,
		},
	}
	poolMgr := pool.NewManager(pools, log)
	defer poolMgr.Stop()

	// Create monitor
	monCfg := config.MonitorConfig{Enabled: true, UpdateIntervalSeconds: 10}
	mon := monitor.NewMonitor(monCfg, log)

	miner := NewMiner(cfg, poolMgr, mon, log)
	if miner == nil {
		t.Fatal("Failed to create miner")
	}

	if len(miner.workers) != 2 {
		t.Errorf("Expected 2 workers, got %d", len(miner.workers))
	}

	if miner.config.Threads != 2 {
		t.Errorf("Expected 2 threads, got %d", miner.config.Threads)
	}
}

func TestMinerStartStop(t *testing.T) {
	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"ok"}`))
		}
	}))
	defer server.Close()

	log, _ := logger.New(logger.Config{Level: "info"})
	
	cfg := &MinerConfig{Threads: 1}
	
	pools := []config.PoolConfig{
		{
			Name:     "test-pool",
			URL:      server.URL,
			Address:  "NOGO006f44f4319250563c65919062932cc1cd7bae04045c355bf53bcb9d7f785c0b473fabfd7c",
			Priority: 1,
			Enabled:  true,
		},
	}
	poolMgr := pool.NewManager(pools, log)
	
	monCfg := config.MonitorConfig{Enabled: true, UpdateIntervalSeconds: 1}
	mon := monitor.NewMonitor(monCfg, log)

	miner := NewMiner(cfg, poolMgr, mon, log)

	// Start miner
	if err := miner.Start(); err != nil {
		t.Fatalf("Failed to start miner: %v", err)
	}

	if !miner.IsRunning() {
		t.Error("Expected miner to be running")
	}

	// Let it run briefly
	time.Sleep(200 * time.Millisecond)

	// Stop miner
	if err := miner.Stop(); err != nil {
		t.Fatalf("Failed to stop miner: %v", err)
	}

	if miner.IsRunning() {
		t.Error("Expected miner to be stopped")
	}
}

func TestGetHashRate(t *testing.T) {
	log, _ := logger.New(logger.Config{Level: "info"})
	cfg := &MinerConfig{Threads: 2}
	
	pools := []config.PoolConfig{{Name: "test", URL: "http://invalid", Address: "NOGO" + "00000000000000000000000000000000000000000000000000000000000000000000000000", Priority: 1, Enabled: true}}
	poolMgr := pool.NewManager(pools, log)
	defer poolMgr.Stop()
	
	monCfg := config.MonitorConfig{Enabled: true}
	mon := monitor.NewMonitor(monCfg, log)

	miner := NewMiner(cfg, poolMgr, mon, log)

	hashRate := miner.GetHashRate()
	if hashRate != 0 {
		t.Errorf("Expected hash rate 0 before start, got %d", hashRate)
	}
}

func TestGetStats(t *testing.T) {
	log, _ := logger.New(logger.Config{Level: "info"})
	cfg := &MinerConfig{Threads: 2}
	
	pools := []config.PoolConfig{{Name: "test", URL: "http://invalid", Address: "NOGO" + "00000000000000000000000000000000000000000000000000000000000000000000000000", Priority: 1, Enabled: true}}
	poolMgr := pool.NewManager(pools, log)
	defer poolMgr.Stop()
	
	monCfg := config.MonitorConfig{Enabled: true}
	mon := monitor.NewMonitor(monCfg, log)

	miner := NewMiner(cfg, poolMgr, mon, log)

	stats := miner.GetStats()
	if stats == nil {
		t.Fatal("Expected stats to be non-nil")
	}

	if stats.ActiveWorkers != 2 {
		t.Errorf("Expected 2 active workers, got %d", stats.ActiveWorkers)
	}

	if stats.Running {
		t.Error("Expected miner not running")
	}
}

func TestUpdateJob(t *testing.T) {
	log, _ := logger.New(logger.Config{Level: "info"})
	cfg := &MinerConfig{Threads: 1}
	
	pools := []config.PoolConfig{{Name: "test", URL: "http://invalid", Address: "NOGO" + "00000000000000000000000000000000000000000000000000000000000000000000000000", Priority: 1, Enabled: true}}
	poolMgr := pool.NewManager(pools, log)
	defer poolMgr.Stop()
	
	monCfg := config.MonitorConfig{Enabled: true}
	mon := monitor.NewMonitor(monCfg, log)

	miner := NewMiner(cfg, poolMgr, mon, log)

	template := &nogopow.BlockHeader{
		Height:       100,
		PrevHash:     make([]byte, 32),
		MerkleRoot:   make([]byte, 32),
		Timestamp:    time.Now().Unix(),
		Difficulty:   big.NewInt(1 << 18),
		MinerAddress: []byte("NOGO006f44f4319250563c65919062932cc1cd7bae04045c355bf53bcb9d7f785c0b473fabfd7c"),
		ChainID:      1,
	}

	miner.UpdateJob(template, "target")

	// Verify job is set
	miner.jobMu.RLock()
	job := miner.currentJob
	miner.jobMu.RUnlock()

	if job == nil {
		t.Fatal("Expected job to be set")
	}

	if job.Template.Height != 100 {
		t.Errorf("Expected height 100, got %d", job.Template.Height)
	}
}

func TestSetThreads(t *testing.T) {
	log, _ := logger.New(logger.Config{Level: "info"})
	cfg := &MinerConfig{Threads: 2}
	
	pools := []config.PoolConfig{{Name: "test", URL: "http://invalid", Address: "NOGO" + "00000000000000000000000000000000000000000000000000000000000000000000000000", Priority: 1, Enabled: true}}
	poolMgr := pool.NewManager(pools, log)
	defer poolMgr.Stop()
	
	monCfg := config.MonitorConfig{Enabled: true}
	mon := monitor.NewMonitor(monCfg, log)

	miner := NewMiner(cfg, poolMgr, mon, log)

	// Change threads before start
	if err := miner.SetThreads(4); err != nil {
		t.Errorf("Failed to set threads: %v", err)
	}

	if len(miner.workers) != 4 {
		t.Errorf("Expected 4 workers, got %d", len(miner.workers))
	}

	if miner.config.Threads != 4 {
		t.Errorf("Expected 4 threads, got %d", miner.config.Threads)
	}
}

func TestPauseResume(t *testing.T) {
	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"ok"}`))
		}
	}))
	defer server.Close()

	log, _ := logger.New(logger.Config{Level: "info"})
	cfg := &MinerConfig{Threads: 1}
	
	pools := []config.PoolConfig{
		{
			Name:     "test-pool",
			URL:      server.URL,
			Address:  "NOGO006f44f4319250563c65919062932cc1cd7bae04045c355bf53bcb9d7f785c0b473fabfd7c",
			Priority: 1,
			Enabled:  true,
		},
	}
	poolMgr := pool.NewManager(pools, log)
	
	monCfg := config.MonitorConfig{Enabled: true}
	mon := monitor.NewMonitor(monCfg, log)

	miner := NewMiner(cfg, poolMgr, mon, log)

	// Start miner
	if err := miner.Start(); err != nil {
		t.Fatalf("Failed to start miner: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	// Pause miner
	if err := miner.Pause(); err != nil {
		t.Errorf("Failed to pause miner: %v", err)
	}

	// Give workers time to stop
	time.Sleep(100 * time.Millisecond)

	activeWorkers := miner.GetActiveWorkers()
	if activeWorkers != 0 {
		t.Logf("Expected 0 active workers after pause, got %d", activeWorkers)
	}

	// Resume miner
	if err := miner.Resume(); err != nil {
		t.Errorf("Failed to resume miner: %v", err)
	}

	// Stop miner
	if err := miner.Stop(); err != nil {
		t.Fatalf("Failed to stop miner: %v", err)
	}
}

func TestGetActiveWorkers(t *testing.T) {
	log, _ := logger.New(logger.Config{Level: "info"})
	cfg := &MinerConfig{Threads: 2}
	
	pools := []config.PoolConfig{{Name: "test", URL: "http://invalid", Address: "NOGO" + "00000000000000000000000000000000000000000000000000000000000000000000000000", Priority: 1, Enabled: true}}
	poolMgr := pool.NewManager(pools, log)
	defer poolMgr.Stop()
	
	monCfg := config.MonitorConfig{Enabled: true}
	mon := monitor.NewMonitor(monCfg, log)

	miner := NewMiner(cfg, poolMgr, mon, log)

	activeWorkers := miner.GetActiveWorkers()
	if activeWorkers != 0 {
		t.Errorf("Expected 0 active workers before start, got %d", activeWorkers)
	}
}

func TestMinerConfigDefaults(t *testing.T) {
	log, _ := logger.New(logger.Config{Level: "info"})
	cfg := &MinerConfig{Threads: 0} // Should default to CPU count
	
	pools := []config.PoolConfig{{Name: "test", URL: "http://invalid", Address: "NOGO" + "00000000000000000000000000000000000000000000000000000000000000000000000000", Priority: 1, Enabled: true}}
	poolMgr := pool.NewManager(pools, log)
	defer poolMgr.Stop()
	
	monCfg := config.MonitorConfig{Enabled: true}
	mon := monitor.NewMonitor(monCfg, log)

	miner := NewMiner(cfg, poolMgr, mon, log)

	if miner.config.Threads <= 0 {
		t.Error("Expected threads to be set to CPU count")
	}

	t.Logf("Default threads: %d", miner.config.Threads)
}

func TestWorkerMine(t *testing.T) {
	worker := &Worker{
		id:     0,
		engine: nogopow.NewEngine(),
		stopCh: make(chan struct{}),
	}

	template := &nogopow.BlockHeader{
		Height:       1,
		PrevHash:     make([]byte, 32),
		MerkleRoot:   make([]byte, 32),
		Timestamp:    time.Now().Unix(),
		Difficulty:   big.NewInt(1 << 20),
		MinerAddress: []byte("NOGO006f44f4319250563c65919062932cc1cd7bae04045c355bf53bcb9d7f785c0b473fabfd7c"),
		ChainID:      1,
	}

	job := &MiningJob{
		Template:  template,
		StartTime: time.Now(),
		Target:    "target",
	}

	// Mine for a short time
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	done := make(chan *nogopow.MiningResult)
	go func() {
		result := worker.mine(job)
		done <- result
	}()

	select {
	case result := <-done:
		if result == nil {
			t.Log("No result returned (timeout or no solution)")
		} else {
			t.Logf("Mining result: hashes=%d, success=%v", result.HashesTried, result.Success)
		}
	case <-ctx.Done():
		t.Log("Mining timed out (expected)")
	}
}
