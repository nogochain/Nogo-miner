// Package pool provides mining pool management
package pool

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/nogochain/nogo-miner/internal/config"
	"github.com/nogochain/nogo-miner/internal/logger"
	"github.com/nogochain/nogo-miner/internal/stratum"
)

// Pool represents a mining pool connection
type Pool struct {
	config     config.PoolConfig
	client     *stratum.Client
	connected  bool
	lastPing   time.Time
	hashRate   uint64
	accepted   uint64
	rejected   uint64
	mu         sync.RWMutex
}

// Manager manages multiple mining pools
type Manager struct {
	mu            sync.RWMutex
	pools         []*Pool
	currentPool   int
	log           *logger.Logger
	healthCheckCh chan struct{}
	stopCh        chan struct{}
}

// PoolStats represents pool statistics
type PoolStats struct {
	Name       string
	URL        string
	Connected  bool
	HashRate   uint64
	Accepted   uint64
	Rejected   uint64
	LastPing   time.Time
	Priority   int
}

// NewManager creates a new pool manager
func NewManager(cfgs []config.PoolConfig, log *logger.Logger) *Manager {
	if log == nil {
		log, _ = logger.New(logger.Config{Level: "info"})
	}

	manager := &Manager{
		pools:         make([]*Pool, 0, len(cfgs)),
		currentPool:   -1,
		log:           log,
		healthCheckCh: make(chan struct{}),
		stopCh:        make(chan struct{}),
	}

	// Initialize pools
	for _, poolCfg := range cfgs {
		if !poolCfg.Enabled {
			continue
		}

		pool := &Pool{
			config:    poolCfg,
			connected: false,
		}

		// Create Stratum client for this pool (use WSURL for WebSocket connection)
		poolURL := poolCfg.WSURL
		if poolURL == "" {
			poolURL = poolCfg.URL
		}

		manager.log.Infof("Initializing pool: %s, address: %s, url: %s", poolCfg.Name, poolCfg.Address, poolURL)
		pool.client = stratum.NewClient(poolURL, poolCfg.Address, log)
		manager.pools = append(manager.pools, pool)
	}

	// Sort pools by priority
	manager.sortPoolsByPriority()

	return manager
}

// sortPoolsByPriority sorts pools by priority (lower = higher priority)
func (m *Manager) sortPoolsByPriority() {
	for i := 0; i < len(m.pools); i++ {
		for j := i + 1; j < len(m.pools); j++ {
			if m.pools[j].config.Priority < m.pools[i].config.Priority {
				m.pools[i], m.pools[j] = m.pools[j], m.pools[i]
			}
		}
	}
}

// Start starts the pool manager
func (m *Manager) Start(ctx context.Context) error {
	m.log.Infof("Starting pool manager with %d pools", len(m.pools))

	if len(m.pools) == 0 {
		return fmt.Errorf("no enabled pools")
	}

	// Connect to the first pool
	if err := m.connectToPool(ctx, 0); err != nil {
		m.log.Errorf("Failed to connect to initial pool: %v", err)
		// Try to connect to other pools
		for i := 1; i < len(m.pools); i++ {
			if err := m.connectToPool(ctx, i); err == nil {
				break
			}
		}
	}

	// Start health check goroutine
	go m.healthCheckLoop(ctx)

	return nil
}

// connectToPool connects to a specific pool
// P1 Issue 3.1 FIX: Add proper locking to prevent connection race
func (m *Manager) connectToPool(ctx context.Context, index int) error {
	if index < 0 || index >= len(m.pools) {
		return fmt.Errorf("invalid pool index: %d", index)
	}

	pool := m.pools[index]
	
	// Lock the pool to prevent concurrent connection attempts
	pool.mu.Lock()
	if pool.connected {
		pool.mu.Unlock()
		return fmt.Errorf("already connected to pool %s", pool.config.Name)
	}
	pool.mu.Unlock()

	m.log.Infof("Connecting to pool: %s (%s)", pool.config.Name, pool.config.URL)

	// Connect with timeout
	connectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := pool.client.Connect(connectCtx); err != nil {
		return fmt.Errorf("connect failed: %w", err)
	}

	// Update connection state with locking
	pool.mu.Lock()
	pool.connected = true
	pool.lastPing = time.Now()
	pool.mu.Unlock()

	m.mu.Lock()
	m.currentPool = index
	m.mu.Unlock()

	m.log.Infof("Connected to pool: %s", pool.config.Name)
	return nil
}

// healthCheckLoop periodically checks pool health
func (m *Manager) healthCheckLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.checkPoolHealth(ctx)
		}
	}
}

// checkPoolHealth checks the current pool health
func (m *Manager) checkPoolHealth(ctx context.Context) {
	m.mu.RLock()
	if m.currentPool < 0 || m.currentPool >= len(m.pools) {
		m.mu.RUnlock()
		return
	}

	currentPool := m.pools[m.currentPool]
	m.mu.RUnlock()

	// Check if still connected
	if !currentPool.client.IsConnected() {
		m.log.Warnf("Current pool %s disconnected", currentPool.config.Name)
		m.switchToNextPool(ctx)
		return
	}

	currentPool.mu.Lock()
	currentPool.lastPing = time.Now()
	currentPool.mu.Unlock()
}

// switchToNextPool switches to the next available pool
// P1 Issue 3.1: Connection race in switchToNextPool
// CRITICAL FIX: Add proper locking to prevent connection race
func (m *Manager) switchToNextPool(ctx context.Context) {
	m.mu.Lock()
	
	// CRITICAL FIX: Store current pool index BEFORE releasing lock
	currentIdx := m.currentPool
	m.mu.Unlock()
	
	// Disconnect from current pool with proper locking
	if currentIdx >= 0 && currentIdx < len(m.pools) {
		currentPool := m.pools[currentIdx]
		
		// Lock the pool to prevent concurrent connection attempts
		currentPool.mu.Lock()
		if currentPool.connected {
			currentPool.connected = false
			// Close client connection
			if currentPool.client != nil {
				currentPool.client.Close()
			}
		}
		currentPool.mu.Unlock()
	}
	
	// Try next pools
	for i := 0; i < len(m.pools); i++ {
		poolIndex := (currentIdx + 1 + i) % len(m.pools)

		// Skip current pool only when there are other pools to try.
		// When only one pool is configured, reconnect to the same pool
		// instead of falling through to currentPool = -1.
		if poolIndex == currentIdx && len(m.pools) > 1 {
			continue
		}
		
		targetPool := m.pools[poolIndex]
		
		// Lock the target pool to prevent concurrent connection
		targetPool.mu.Lock()
		if targetPool.connected {
			// Already connected, just switch
			targetPool.mu.Unlock()
			
			m.mu.Lock()
			m.currentPool = poolIndex
			m.mu.Unlock()
			
			m.log.Infof("Switched to already connected pool: %s", targetPool.config.Name)
			return
		}
		targetPool.mu.Unlock()
		
		m.log.Infof("Trying to connect to pool: %s", targetPool.config.Name)
		
		// Connect with timeout
		connectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		
		// Use a channel to prevent concurrent dial
		connectCh := make(chan error, 1)
		go func() {
			connectCh <- targetPool.client.Connect(connectCtx)
		}()
		
		err := <-connectCh
		
		if err == nil {
			// Update connection state with locking
			targetPool.mu.Lock()
			targetPool.connected = true
			targetPool.lastPing = time.Now()
			targetPool.mu.Unlock()
			
			m.mu.Lock()
			m.currentPool = poolIndex
			m.mu.Unlock()
			
			m.log.Infof("✅ Switched to pool: %s", targetPool.config.Name)
			return
		}
		
		m.log.Warnf("Failed to connect to pool %s: %v", targetPool.config.Name, err)
	}
	
	m.mu.Lock()
	m.currentPool = -1
	m.mu.Unlock()
	m.log.Error("No available pools")
}

// GetCurrentPool returns the current pool
func (m *Manager) GetCurrentPool() *Pool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.currentPool < 0 || m.currentPool >= len(m.pools) {
		return nil
	}

	return m.pools[m.currentPool]
}

// GetClient returns the Stratum client for the current pool
func (m *Manager) GetClient() *stratum.Client {
	pool := m.GetCurrentPool()
	if pool == nil {
		return nil
	}
	return pool.client
}

// GetAddress returns the mining address for the current pool
func (m *Manager) GetAddress() string {
	pool := m.GetCurrentPool()
	if pool == nil {
		return ""
	}
	return pool.config.Address
}

// RecordShare records a share submission
func (m *Manager) RecordShare(accepted bool) {
	pool := m.GetCurrentPool()
	if pool == nil {
		return
	}

	pool.mu.Lock()
	defer pool.mu.Unlock()

	if accepted {
		pool.accepted++
	} else {
		pool.rejected++
	}
}

// UpdateHashRate updates the hash rate for the current pool
func (m *Manager) UpdateHashRate(hashRate uint64) {
	pool := m.GetCurrentPool()
	if pool == nil {
		return
	}

	pool.mu.Lock()
	defer pool.mu.Unlock()
	pool.hashRate = hashRate
}

// GetStats returns statistics for all pools
func (m *Manager) GetStats() []PoolStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := make([]PoolStats, 0, len(m.pools))
	for _, pool := range m.pools {
		pool.mu.RLock()
		stats = append(stats, PoolStats{
			Name:      pool.config.Name,
			URL:       pool.config.URL,
			Connected: pool.connected,
			HashRate:  pool.hashRate,
			Accepted:  pool.accepted,
			Rejected:  pool.rejected,
			LastPing:  pool.lastPing,
			Priority:  pool.config.Priority,
		})
		pool.mu.RUnlock()
	}

	return stats
}

// IsConnected returns whether connected to any pool
func (m *Manager) IsConnected() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.currentPool >= 0 && m.currentPool >= 0 && m.currentPool < len(m.pools) && m.pools[m.currentPool].connected
}

// GetPoolCount returns the number of configured pools
func (m *Manager) GetPoolCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.pools)
}

// Stop stops the pool manager
func (m *Manager) Stop() {
	close(m.stopCh)

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, pool := range m.pools {
		if pool.connected {
			pool.client.Close()
			pool.connected = false
		}
	}

	// Reset current pool index
	m.currentPool = -1

	m.log.Info("Pool manager stopped")
}
