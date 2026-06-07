// Package miner provides the core mining logic
package miner

import (
	"context"
	"encoding/hex"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nogochain/nogo-miner/internal/logger"
	"github.com/nogochain/nogo-miner/internal/monitor"
	"github.com/nogochain/nogo-miner/internal/pool"
	"github.com/nogochain/nogo-miner/internal/stratum"
	"github.com/nogochain/nogo-miner/pkg/nogopow"
)

// Worker represents a mining worker
type Worker struct {
	id        int
	engine    *nogopow.Engine
	stopCh    chan struct{}
	isMining  int32
	hashCount uint64
}

// Miner represents the main miner
type Miner struct {
	mu            sync.RWMutex
	config        *MinerConfig
	workers       []*Worker
	poolManager   *pool.Manager
	monitor       *monitor.Monitor
	log           *logger.Logger
	ctx           context.Context
	cancel        context.CancelFunc
	running       bool
	currentJob    *MiningJob
	jobMu         sync.RWMutex
	submittedWork uint64
	acceptedWork  uint64
}

// MinerConfig represents miner configuration
type MinerConfig struct {
	Threads         int
	BatchSize       int
	ShareDifficulty int
}

// MiningJob represents a mining job
type MiningJob struct {
	Template   *nogopow.BlockHeader
	StartTime  time.Time
	Target     string
	JobID      string
	ExtraNonce string
	JobIDNum   uint64
}

// NewMiner creates a new miner
func NewMiner(cfg *MinerConfig, poolMgr *pool.Manager, mon *monitor.Monitor, log *logger.Logger) *Miner {
	if log == nil {
		log, _ = logger.New(logger.Config{Level: "info"})
	}

	if cfg.Threads <= 0 {
		cfg.Threads = runtime.NumCPU()
	}

	ctx, cancel := context.WithCancel(context.Background())

	miner := &Miner{
		config:      cfg,
		poolManager: poolMgr,
		monitor:     mon,
		log:         log,
		ctx:         ctx,
		cancel:      cancel,
		workers:     make([]*Worker, 0, cfg.Threads),
	}

	// Create workers
	for i := 0; i < cfg.Threads; i++ {
		worker := &Worker{
			id:     i,
			engine: nogopow.NewEngine(),
			stopCh: make(chan struct{}),
		}
		miner.workers = append(miner.workers, worker)
	}

	return miner
}

// Start starts the miner
func (m *Miner) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		return fmt.Errorf("miner already running")
	}

	m.log.Infof("Starting miner with %d threads", len(m.workers))

	// Start pool manager
	if err := m.poolManager.Start(m.ctx); err != nil {
		return fmt.Errorf("start pool manager: %w", err)
	}

	// Start workers
	for _, worker := range m.workers {
		go m.runWorker(worker)
	}

	// Start job monitor
	go m.monitorJobs()

	m.running = true
	m.log.Info("Miner started")

	return nil
}

// runWorker runs a mining worker
func (m *Miner) runWorker(worker *Worker) {
	m.log.Debugf("Worker %d started", worker.id)

	for {
		select {
		case <-m.ctx.Done():
			select {
			case <-worker.stopCh:
			default:
				close(worker.stopCh)
			}
			m.log.Debugf("Worker %d stopped", worker.id)
			return
		default:
			// Get current job - capture the specific job this worker will mine on.
			// CRITICAL: The worker must pass this exact job reference through to
			// submitSolution so the share is submitted with the correct jobId.
			// Reading m.currentJob again at submission time is a race condition
			// because a new job may have arrived while the worker was mining.
			m.jobMu.RLock()
			job := m.currentJob
			m.jobMu.RUnlock()

			if job == nil {
				// No job, wait
				time.Sleep(100 * time.Millisecond)
				continue
			}

			// Mine with this specific job
			result := worker.mine(job)

			if result != nil && result.Success {
				m.handleSuccess(worker, result, job)
			}
		}
	}
}

// mine performs mining on a job
func (w *Worker) mine(job *MiningJob) *nogopow.MiningResult {
	atomic.StoreInt32(&w.isMining, 1)
	defer atomic.StoreInt32(&w.isMining, 0)

	// No timeout — mine continuously until solution found.
	// Between blocks, the Pool only sends jobs for actual new blocks (not difficulty
	// changes). The engine continues scanning nonces uninterrupted.
	mineStopCh := make(chan struct{}) // never closed, engine runs until solution
	// CRITICAL: Must include StateRoot to match node's Header.Root (state root)
	// Note: TxHash is calculated from MerkleRoot inside engine.Mine()
	result := w.engine.Mine(&nogopow.BlockHeader{
		Height:       job.Template.Height,
		PrevHash:     job.Template.PrevHash,
		MerkleRoot:   job.Template.MerkleRoot,
		StateRoot:    job.Template.StateRoot, // State root for PoW calculation
		Timestamp:    job.Template.Timestamp,
		Difficulty:   job.Template.Difficulty,
		MinerAddress: job.Template.MinerAddress,
		ChainID:      job.Template.ChainID,
	}, mineStopCh)

	if result == nil {
		return nil
	}

	// Update hash count
	atomic.AddUint64(&w.hashCount, result.HashesTried)

	return result
}

// logHashRate logs the hash rate for debugging
func (w *Worker) logHashRate(hashRate float64, duration time.Duration, hashes uint64) {
	// Only log periodically to avoid spam (every 10 seconds)
	if duration >= 10*time.Second {
		fmt.Printf("[Worker %d] Hashrate: %.2f H/s | Hashes: %d | Duration: %v\n",
			w.id, hashRate, hashes, duration)
	}
}

// handleSuccess handles a successful mining result
func (m *Miner) handleSuccess(worker *Worker, result *nogopow.MiningResult, job *MiningJob) {
	m.log.Infof("Worker %d found solution! Nonce: %d, Hashes: %d, JobID: %d",
		worker.id, result.Nonce, result.HashesTried, job.JobIDNum)

	// Submit to pool using the specific job the nonce was computed against.
	// CRITICAL: Use the captured job reference, NOT m.currentJob, to avoid
	// submitting with a different jobId than what the nonce was mined for.
	m.submitSolution(result, job)

	// Listen for submission result
	go m.listenForResult()
}

// listenForResult listens for share submission results
func (m *Miner) listenForResult() {
	client := m.poolManager.GetClient()
	if client == nil {
		return
	}

	resultCh := client.GetResultChannel()

	ctx, cancel := context.WithTimeout(m.ctx, 15*time.Second)
	defer cancel()

	select {
	case result, ok := <-resultCh:
		if !ok {
			return
		}
		if result.Accepted {
			atomic.AddUint64(&m.submittedWork, 1)
			atomic.AddUint64(&m.acceptedWork, 1)
			m.poolManager.RecordShare(true)
			m.monitor.RecordShare(true, "")
			m.log.Infof("Share accepted! JobID: %d", result.JobID)
		} else {
			m.poolManager.RecordShare(false)
			m.monitor.RecordShare(false, result.Message)
			m.log.Warnf("Share rejected: %s", result.Message)
		}
	case <-ctx.Done():
		// Timeout waiting for result
		m.log.Debug("Timeout waiting for share result")
	}
}

// submitSolution submits a solution to the pool using the specific job the nonce was mined for.
// CRITICAL: The `job` parameter must be the exact job reference that was passed to worker.mine(),
// NOT m.currentJob, because m.currentJob may have been updated to a different job while the
// worker was mining. Using the wrong jobId for submission causes the pool to validate the
// share against different parameters, resulting in "invalid PoW" rejection.
func (m *Miner) submitSolution(result *nogopow.MiningResult, job *MiningJob) {
	client := m.poolManager.GetClient()
	if client == nil {
		m.log.Error("No pool client available")
		return
	}

	if job == nil {
		m.log.Warn("No job to submit solution")
		return
	}

	// Submit share to pool via Stratum client
	ctx, cancel := context.WithTimeout(m.ctx, 3*time.Second)
	defer cancel()

	if err := client.SubmitShare(ctx, job.JobIDNum, result.Nonce); err != nil {
		m.log.Errorf("Failed to submit share: %v", err)
		m.poolManager.RecordShare(false)
		m.monitor.RecordShare(false, err.Error())
		return
	}

	m.log.Infof("Share submitted: jobId=%d, nonce=%d", job.JobIDNum, result.Nonce)
}

// monitorJobs monitors and fetches new mining jobs from Stratum client.
// Handles reconnection: if job channel closes (client stopped during pool switch),
// it re-fetches the current client and retries.
func (m *Miner) monitorJobs() {
	// Re-fetch ticker for dynamic client discovery
	refetchTicker := time.NewTicker(15 * time.Second)
	defer refetchTicker.Stop()

	// Job expiry: discard jobs older than 10 minutes to prevent stale mining
	const jobExpiryDuration = 10 * time.Minute

	for {
		client := m.poolManager.GetClient()
		if client == nil {
			m.log.Debug("No pool client available for job fetch, retrying in 15s")
			select {
			case <-m.ctx.Done():
				return
			case <-refetchTicker.C:
				continue
			}
		}

		jobCh := client.GetJobChannel()

		m.log.Debugf("monitorJobs: listening on job channel")

	innerLoop:
		for {
			select {
			case <-m.ctx.Done():
				return
			case <-refetchTicker.C:
				// Periodically check if the client has changed (pool switch)
				currentClient := m.poolManager.GetClient()
				if currentClient != client {
					m.log.Debugf("monitorJobs: pool client changed, re-fetching job channel")
					break innerLoop
				}
			case job, ok := <-jobCh:
				if !ok {
					m.log.Warn("Job channel closed, re-fetching client")
					break innerLoop
				}

				// Discard stale jobs beyond expiry threshold
				if time.Since(time.Unix(job.Timestamp, 0)) > jobExpiryDuration {
					m.log.Debugf("Skipping expired job: height=%d, timestamp=%d", job.Height, job.Timestamp)
					continue
				}

				m.handleNewJob(job)
			}
		}
	}
}

// handleNewJob processes a new mining job from pool
func (m *Miner) handleNewJob(job *stratum.MiningJob) {
	m.log.Infof("Received new mining job: height=%d, jobId=%d, difficulty=%d, stateRoot=%s",
		job.Height, job.JobID, job.Difficulty, job.StateRoot)

	// Convert hex strings to byte slices
	prevHash, err := hex.DecodeString(job.PrevHash)
	if err != nil {
		m.log.Errorf("Failed to decode prevHash: %v", err)
		return
	}

	merkleRoot, err := hex.DecodeString(job.MerkleRoot)
	if err != nil {
		m.log.Errorf("Failed to decode merkleRoot: %v", err)
		return
	}

	// Decode stateRoot (World State MPT root hash, REQUIRED for PoW)
	// CRITICAL: StateRoot must NOT be empty - PoW verification will fail
	var stateRoot []byte
	if job.StateRoot != "" {
		var err error
		stateRoot, err = hex.DecodeString(job.StateRoot)
		if err != nil {
			m.log.Errorf("❌ CRITICAL: Failed to decode stateRoot: %v", err)
			m.log.Errorf("❌ Cannot mine without valid stateRoot - PoW verification will fail!")
			return // ❌ REFUSE to mine - stateRoot is invalid
		}
		// Validate stateRoot length (must be 32 bytes for Keccak-256 hash)
		if len(stateRoot) != 32 {
			m.log.Errorf("❌ CRITICAL: Invalid stateRoot length: %d bytes (expected 32)", len(stateRoot))
			m.log.Errorf("❌ Cannot mine without valid stateRoot - PoW verification will fail!")
			return // ❌ REFUSE to mine - stateRoot is invalid
		}
		m.log.Infof("✅ StateRoot decoded successfully: %x...", stateRoot[:8])
	} else {
		// ❌ CRITICAL: StateRoot is EMPTY - refuse to mine
		m.log.Errorf("❌ CRITICAL: StateRoot is EMPTY! Pool must send stateRoot for correct PoW calculation.")
		m.log.Errorf("❌ Cannot mine without stateRoot - PoW verification will fail!")
		m.log.Errorf("❌ Possible causes:")
		m.log.Errorf("❌   1. Pool is not sending stateRoot (bug in NogoPool)")
		m.log.Errorf("❌   2. Node is not calculating stateRoot (bug in NogoChain)")
		m.log.Errorf("❌   3. Network error - job data corrupted")
		return // ❌ REFUSE to mine - stateRoot is empty
	}

	// Convert Stratum job to internal mining job
	// Job difficulty is already *big.Int from stratum client
	difficulty := job.Difficulty

	// Convert miner address string to []byte
	minerAddr := []byte(job.MinerAddress)

	template := &nogopow.BlockHeader{
		Height:       job.Height,
		PrevHash:     prevHash,
		MerkleRoot:   merkleRoot,
		StateRoot:    stateRoot, // World State MPT root hash (for PoW)
		Timestamp:    job.Timestamp,
		Difficulty:   difficulty,
		MinerAddress: minerAddr,
		ChainID:      1, // Default chain ID
	}

	m.jobMu.Lock()
	m.currentJob = &MiningJob{
		Template:   template,
		StartTime:  time.Now(),
		Target:     job.Difficulty.String(),
		JobID:      fmt.Sprintf("%d", job.JobID),
		ExtraNonce: job.ExtraNonce,
		JobIDNum:   job.JobID,
	}
	m.jobMu.Unlock()

	m.log.Infof("Updated mining job: height=%d, target=%s", job.Height, job.PrevHash)
}

// Stop stops the miner
func (m *Miner) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return nil
	}

	m.log.Info("Stopping miner...")

	// Cancel context
	m.cancel()

	// Stop workers
	for _, worker := range m.workers {
		select {
		case <-worker.stopCh:
		default:
			close(worker.stopCh)
		}
	}

	// Stop pool manager
	m.poolManager.Stop()

	// Stop monitor
	m.monitor.Stop()

	m.running = false
	m.log.Info("Miner stopped")

	return nil
}

// GetHashRate returns total hash rate
func (m *Miner) GetHashRate() uint64 {
	var total uint64 = 0
	for _, worker := range m.workers {
		total += worker.engine.GetHashRate()
	}
	return total
}

// GetStats returns miner statistics
func (m *Miner) GetStats() *MinerStats {
	var totalHashes uint64 = 0
	for _, worker := range m.workers {
		totalHashes += atomic.LoadUint64(&worker.hashCount)
	}

	return &MinerStats{
		HashRate:      m.GetHashRate(),
		TotalHashes:   totalHashes,
		SubmittedWork: atomic.LoadUint64(&m.submittedWork),
		AcceptedWork:  atomic.LoadUint64(&m.acceptedWork),
		ActiveWorkers: len(m.workers),
		Running:       m.running,
	}
}

// MinerStats represents miner statistics
type MinerStats struct {
	HashRate      uint64
	TotalHashes   uint64
	SubmittedWork uint64
	AcceptedWork  uint64
	ActiveWorkers int
	Running       bool
}

// UpdateJob updates the current mining job
func (m *Miner) UpdateJob(template *nogopow.BlockHeader, target string) {
	m.jobMu.Lock()
	defer m.jobMu.Unlock()

	m.currentJob = &MiningJob{
		Template:  template,
		StartTime: time.Now(),
		Target:    target,
		JobID:     fmt.Sprintf("job-%d", time.Now().Unix()),
	}

	m.log.Debugf("Updated mining job: height=%d", template.Height)
}

// SetThreads sets the number of mining threads
func (m *Miner) SetThreads(threads int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		return fmt.Errorf("cannot change threads while mining")
	}

	if threads <= 0 {
		threads = runtime.NumCPU()
	}

	// Adjust worker count
	currentWorkers := len(m.workers)
	if threads > currentWorkers {
		// Add workers
		for i := currentWorkers; i < threads; i++ {
			worker := &Worker{
				id:     i,
				engine: nogopow.NewEngine(),
				stopCh: make(chan struct{}),
			}
			m.workers = append(m.workers, worker)
		}
	} else if threads < currentWorkers {
		// Remove workers
		m.workers = m.workers[:threads]
	}

	m.config.Threads = threads
	m.log.Infof("Set threads to %d", threads)

	return nil
}

// Pause pauses mining
func (m *Miner) Pause() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return fmt.Errorf("miner not running")
	}

	m.log.Info("Pausing mining...")

	// Stop all workers
	for _, worker := range m.workers {
		close(worker.stopCh)
		worker.stopCh = make(chan struct{})
	}

	return nil
}

// Resume resumes mining
func (m *Miner) Resume() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return fmt.Errorf("miner not running")
	}

	m.log.Info("Resuming mining...")

	// Restart workers
	for _, worker := range m.workers {
		go m.runWorker(worker)
	}

	return nil
}

// IsRunning returns whether the miner is running
func (m *Miner) IsRunning() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.running
}

// GetActiveWorkers returns number of active workers
func (m *Miner) GetActiveWorkers() int {
	var active int
	for _, worker := range m.workers {
		if atomic.LoadInt32(&worker.isMining) == 1 {
			active++
		}
	}
	return active
}
