// Package monitor provides mining monitoring and statistics
package monitor

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/nogochain/nogo-miner/internal/config"
	"github.com/nogochain/nogo-miner/internal/logger"
)

// Stats represents mining statistics
type Stats struct {
	mu                sync.RWMutex
	StartTime         time.Time
	HashRate          uint64
	AvgHashRate       uint64
	TotalHashes       uint64
	AcceptedShares    uint64
	RejectedShares    uint64
	InvalidShares     uint64
	TotalReward       float64
	EstimatedReward   float64
	BestShare         string
	ActiveWorkers     int
	PoolName          string
	PoolURL           string
	Uptime            time.Duration
	Temperature       float64
	FanSpeed          int
	PowerConsumption  float64
}

// Monitor represents the monitoring system
type Monitor struct {
	mu          sync.RWMutex
	config      config.MonitorConfig
	stats       *Stats
	log         *logger.Logger
	stopCh      chan struct{}
	updateCh    chan struct{}
	callbacks   []func(*Stats)
	displayFunc func(*Stats)
}

// NewMonitor creates a new monitor
func NewMonitor(cfg config.MonitorConfig, log *logger.Logger) *Monitor {
	if log == nil {
		log, _ = logger.New(logger.Config{Level: "info"})
	}

	monitor := &Monitor{
		config:   cfg,
		stats: &Stats{
			StartTime: time.Now(),
		},
		log:        log,
		stopCh:     make(chan struct{}),
		updateCh:   make(chan struct{}, 10),
		callbacks:  make([]func(*Stats), 0),
	}

	return monitor
}

// Start starts the monitoring system
func (m *Monitor) Start(ctx context.Context) {
	m.log.Info("Starting monitoring system")

	if m.config.Enabled {
		go m.updateLoop(ctx)
		go m.displayLoop(ctx)
	}
}

// updateLoop periodically updates statistics
func (m *Monitor) updateLoop(ctx context.Context) {
	ticker := time.NewTicker(m.config.GetUpdateInterval())
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.updateStats()
		case <-m.updateCh:
			m.updateStats()
		}
	}
}

// displayLoop periodically displays statistics
func (m *Monitor) displayLoop(ctx context.Context) {
	ticker := time.NewTicker(m.config.GetUpdateInterval())
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.display()
		}
	}
}

// updateStats updates mining statistics
func (m *Monitor) updateStats() {
	m.stats.mu.Lock()
	defer m.stats.mu.Unlock()

	// Update uptime
	m.stats.Uptime = time.Since(m.stats.StartTime)

	elapsed := m.stats.Uptime.Seconds()
	if elapsed <= 0 {
		return
	}

	// Calculate real-time hash rate (based on total hashes since start)
	if m.stats.TotalHashes > 0 {
		m.stats.HashRate = uint64(float64(m.stats.TotalHashes) / elapsed)
	}

	// Calculate average hash rate (same as real-time for cumulative calculation)
	if m.stats.TotalHashes > 0 {
		m.stats.AvgHashRate = uint64(float64(m.stats.TotalHashes) / elapsed)
	}

	// Calculate estimated reward (simplified)
	// In production: would use network difficulty and pool statistics
	m.stats.EstimatedReward = float64(m.stats.AcceptedShares) * 0.0001
}

// display displays current statistics
func (m *Monitor) display() {
	m.stats.mu.RLock()
	stats := *m.stats
	m.stats.mu.RUnlock()

	// Call display function if set
	if m.displayFunc != nil {
		m.displayFunc(&stats)
	}

	// Log statistics with detailed hashrate information
	m.log.Infof("📊 Hashrate: %s (Avg: %s) | Accepted: %d | Rejected: %d | Invalid: %d | Uptime: %s | Workers: %d",
		formatHashRate(stats.HashRate),
		formatHashRate(stats.AvgHashRate),
		stats.AcceptedShares,
		stats.RejectedShares,
		stats.InvalidShares,
		formatDuration(stats.Uptime),
		stats.ActiveWorkers)
}

// UpdateHashRate updates the hash rate
func (m *Monitor) UpdateHashRate(hashRate uint64) {
	m.stats.mu.Lock()
	defer m.stats.mu.Unlock()
	m.stats.HashRate = hashRate
}

// AddHashes adds to total hashes
func (m *Monitor) AddHashes(count uint64) {
	m.stats.mu.Lock()
	defer m.stats.mu.Unlock()
	m.stats.TotalHashes += count
}

// RecordShare records a share submission
func (m *Monitor) RecordShare(accepted bool, shareDiff string) {
	m.stats.mu.Lock()
	defer m.stats.mu.Unlock()

	if accepted {
		m.stats.AcceptedShares++
	} else {
		m.stats.RejectedShares++
	}

	// Update best share
	if shareDiff != "" && (m.stats.BestShare == "" || shareDiff < m.stats.BestShare) {
		m.stats.BestShare = shareDiff
	}
}

// UpdatePoolInfo updates pool information
func (m *Monitor) UpdatePoolInfo(name, url string) {
	m.stats.mu.Lock()
	defer m.stats.mu.Unlock()
	m.stats.PoolName = name
	m.stats.PoolURL = url
}

// UpdateWorkers updates active worker count
func (m *Monitor) UpdateWorkers(count int) {
	m.stats.mu.Lock()
	defer m.stats.mu.Unlock()
	m.stats.ActiveWorkers = count
}

// UpdateHardware updates hardware statistics
func (m *Monitor) UpdateHardware(temp float64, fanSpeed int, power float64) {
	m.stats.mu.Lock()
	defer m.stats.mu.Unlock()
	m.stats.Temperature = temp
	m.stats.FanSpeed = fanSpeed
	m.stats.PowerConsumption = power
}

// AddReward adds to total reward
func (m *Monitor) AddReward(amount float64) {
	m.stats.mu.Lock()
	defer m.stats.mu.Unlock()
	m.stats.TotalReward += amount
}

// GetStats returns current statistics
func (m *Monitor) GetStats() *Stats {
	m.stats.mu.RLock()
	defer m.stats.mu.RUnlock()

	stats := *m.stats
	return &stats
}

// RegisterCallback registers a statistics callback
func (m *Monitor) RegisterCallback(cb func(*Stats)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callbacks = append(m.callbacks, cb)
}

// SetDisplayFunc sets the display function
func (m *Monitor) SetDisplayFunc(fn func(*Stats)) {
	m.displayFunc = fn
}

// TriggerUpdate triggers an immediate statistics update
func (m *Monitor) TriggerUpdate() {
	select {
	case m.updateCh <- struct{}{}:
	default:
	}
}

// Stop stops the monitoring system
func (m *Monitor) Stop() {
	close(m.stopCh)
	m.log.Info("Monitoring system stopped")
}

// formatHashRate formats hash rate for display
func formatHashRate(hashRate uint64) string {
	if hashRate >= 1_000_000_000 {
		return fmt.Sprintf("%.2f GH/s", float64(hashRate)/1e9)
	} else if hashRate >= 1_000_000 {
		return fmt.Sprintf("%.2f MH/s", float64(hashRate)/1e6)
	} else if hashRate >= 1_000 {
		return fmt.Sprintf("%.2f KH/s", float64(hashRate)/1e3)
	} else {
		return fmt.Sprintf("%d H/s", hashRate)
	}
}

// formatDuration formats duration for display
func formatDuration(d time.Duration) string {
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60

	if hours > 0 {
		return fmt.Sprintf("%dh %dm %ds", hours, minutes, seconds)
	} else if minutes > 0 {
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	} else {
		return fmt.Sprintf("%ds", seconds)
	}
}

// GetEfficiency returns mining efficiency
func (m *Monitor) GetEfficiency() float64 {
	m.stats.mu.RLock()
	defer m.stats.mu.RUnlock()

	total := m.stats.AcceptedShares + m.stats.RejectedShares
	if total == 0 {
		return 100.0
	}

	return float64(m.stats.AcceptedShares) / float64(total) * 100.0
}

// GetSummary returns a summary string
func (m *Monitor) GetSummary() string {
	m.stats.mu.RLock()
	defer m.stats.mu.RUnlock()

	return fmt.Sprintf(
		"Uptime: %s | Hashrate: %s | Avg: %s | Shares: %dA/%dR | Efficiency: %.1f%%",
		formatDuration(m.stats.Uptime),
		formatHashRate(m.stats.HashRate),
		formatHashRate(m.stats.AvgHashRate),
		m.stats.AcceptedShares,
		m.stats.RejectedShares,
		m.GetEfficiency(),
	)
}

// Reset resets all statistics
func (m *Monitor) Reset() {
	m.stats.mu.Lock()
	defer m.stats.mu.Unlock()

	m.stats.StartTime = time.Now()
	m.stats.HashRate = 0
	m.stats.AvgHashRate = 0
	m.stats.TotalHashes = 0
	m.stats.AcceptedShares = 0
	m.stats.RejectedShares = 0
	m.stats.InvalidShares = 0
	m.stats.TotalReward = 0
	m.stats.EstimatedReward = 0
	m.stats.BestShare = ""
	m.stats.Uptime = 0
}
