// Package stratum provides WebSocket Stratum client for mining pool communication
package stratum

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"net"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// Reconnection backoff parameters
const (
	initialReconnectDelay = 1 * time.Second
	maxReconnectDelay     = 30 * time.Second
	readDeadlineDuration  = 90 * time.Second // Read deadline for silent disconnection detection
)

// Client represents a Stratum client for pool communication
type Client struct {
	mu          sync.RWMutex
	sendMu      sync.Mutex // Protects WebSocket write operations (gorilla/websocket forbids concurrent write)
	conn        *websocket.Conn
	poolURL     string
	minerAddr   string
	log         Logger
	connected   bool
	jobCh       chan *MiningJob
	resultCh    chan *SubmitResult
	lastJob     *MiningJob
	subscribeID string
	loginRespCh chan bool // Channel to wait for login response

	// dialing prevents concurrent dialAndLogin calls (gorilla/websocket forbids concurrent read)
	dialing     int32     // atomic flag: 1 = dialing in progress
	dialDone    chan struct{} // signaled when dialAndLogin completes

	// readLoopOnce ensures the read loop goroutine is started exactly once.
	// After initial Connect, the read loop handles all reconnection internally.
	readLoopStarted bool

	// stopCh is closed when Close() is called to permanently stop the read loop.
	// This prevents the read loop from reconnecting when the pool is deliberately
	// switched away (pool manager switchToNextPool).
	stopCh chan struct{}
}

// MiningJob represents a mining job from pool
type MiningJob struct {
	JobID        uint64   `json:"jobId"`
	Height       uint64   `json:"height"`
	PrevHash     string   `json:"prevHash"`
	MerkleRoot   string   `json:"merkleRoot"`
	StateRoot    string   `json:"stateRoot"` // World State MPT root hash (required for PoW)
	Difficulty   *big.Int `json:"difficulty"` // Changed to *big.Int
	ExtraNonce   string   `json:"extraNonce"`
	Timestamp    int64    `json:"timestamp"`
	MinerAddress string   `json:"minerAddress"`
}

// SubmitResult represents a share submission result
type SubmitResult struct {
	Accepted bool   `json:"accepted"`
	JobID    uint64 `json:"jobId"`
	Message  string `json:"message,omitempty"`
}

// Logger interface for logging
type Logger interface {
	Infof(format string, args ...interface{})
	Debugf(format string, args ...interface{})
	Warnf(format string, args ...interface{})
	Errorf(format string, args ...interface{})
}

// simpleLogger implements Logger interface
type simpleLogger struct{}

func (l *simpleLogger) Infof(format string, args ...interface{})  { fmt.Printf(format+"\n", args...) }
func (l *simpleLogger) Debugf(format string, args ...interface{}) { fmt.Printf(format+"\n", args...) }
func (l *simpleLogger) Warnf(format string, args ...interface{})  { fmt.Printf(format+"\n", args...) }
func (l *simpleLogger) Errorf(format string, args ...interface{}) { fmt.Printf(format+"\n", args...) }

// NewClient creates a new Stratum client
func NewClient(poolURL, minerAddr string, log Logger) *Client {
	if log == nil {
		log = &simpleLogger{}
	}

	return &Client{
		poolURL:     poolURL,
		minerAddr:   minerAddr,
		log:         log,
		jobCh:       make(chan *MiningJob, 10),
		resultCh:    make(chan *SubmitResult, 10),
		loginRespCh: make(chan bool, 1),
		stopCh:      make(chan struct{}),
	}
}

// Connect establishes WebSocket connection to pool.
// On the first call, it also starts the persistent read loop that handles
// auto-reconnection when the connection drops. Subsequent calls are no-ops
// if the read loop is already running.
func (c *Client) Connect(ctx context.Context) error {
	c.mu.Lock()
	if c.readLoopStarted {
		// Read loop already running and handles reconnection internally.
		// If currently disconnected, the read loop will reconnect automatically.
		c.mu.Unlock()
		c.log.Debugf("Connect called but read loop already running, skipping")
		return nil
	}
	c.readLoopStarted = true
	c.mu.Unlock()

	c.log.Infof("Connecting to pool: %s", c.poolURL)

	// Initial connection — dialAndLogin reads WebSocket directly until loginSuccess,
	// because readLoop hasn't started yet.
	log.Printf("[NOGOMINER] Attempting connection to pool: %s", c.poolURL)
	if err := c.dialAndLogin(ctx); err != nil {
		c.log.Errorf("Initial connection failed: %v", err)
		log.Printf("[NOGOMINER] ❌ Connection FAILED: %v", err)
		return err
	}

	log.Printf("[NOGOMINER] ✅ Connection and login SUCCESSFUL!")

	// Start the persistent read loop that handles auto-reconnection.
	// This single goroutine lives for the entire client lifetime.
	// IMPORTANT: Use context.Background() instead of the caller's ctx,
	// because the ctx passed to Connect() is typically a timeout context
	// created by connectToPool (e.g. 10s), which gets cancelled immediately
	// after Connect returns. readLoop must survive for the entire client lifetime.
	go c.readLoop(context.Background())

	return nil
}

// dialAndLogin dials WebSocket and performs login (used for both initial connect and reconnection).
// CRITICAL: This function reads WebSocket messages directly until loginSuccess is received,
// because the readLoop hasn't started yet (initial connect) or is blocked waiting for this
// function to return (reconnection). Without direct reading, no one processes the WebSocket
// response and login would always time out.
//
// IMPORTANT: This function does NOT set c.conn until AFTER loginSuccess is confirmed.
// It uses a local newConn variable throughout the login handshake. This prevents a
// race condition where readLoop picks up the connection mid-login and calls ReadMessage,
// causing a gorilla/websocket panic ("repeated read on failed websocket connection").
func (c *Client) dialAndLogin(ctx context.Context) error {
	// Prevent concurrent dialAndLogin calls (gorilla/websocket forbids concurrent read).
	// If another goroutine is already dialing, wait for it to complete, then check result.
	if !atomic.CompareAndSwapInt32(&c.dialing, 0, 1) {
		// Already dialing — wait for completion, then check if connection succeeded
		c.log.Debugf("dialAndLogin: already dialing, waiting for completion")
		<-c.dialDone
		// After waiting, check if connection is now established
		c.mu.RLock()
		conn := c.conn
		connected := c.connected
		c.mu.RUnlock()
		if conn != nil && connected {
			c.log.Debugf("dialAndLogin: waited for existing dial, connection successful")
			return nil // Connection succeeded, return nil
		}
		// Existing dial failed, return error to let caller retry
		return fmt.Errorf("previous dial failed, please retry")
	}
	// Create fresh dialDone channel for this dial attempt
	c.dialDone = make(chan struct{})
	// Defer: reset dialing flag and close dialDone to signal waiters
	defer func() {
		atomic.StoreInt32(&c.dialing, 0)
		close(c.dialDone)
	}()

	// Parse poolURL to get host
	parsedURL, err := url.Parse(c.poolURL)
	if err != nil {
		return fmt.Errorf("parse pool URL: %w", err)
	}

	poolHost := parsedURL.Host
	if poolHost == "" {
		poolHost = strings.TrimPrefix(parsedURL.Path, "//")
		if idx := strings.Index(poolHost, "/"); idx != -1 {
			poolHost = poolHost[:idx]
		}
	}

	// Test TCP connectivity first
	tcpConn, err := net.DialTimeout("tcp", poolHost, 5*time.Second)
	if err != nil {
		return fmt.Errorf("tcp connect: %w", err)
	}
	tcpConn.Close()

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
		Proxy:            nil,
	}

	newConn, _, err := dialer.Dial(c.poolURL, nil)
	if err != nil {
		return fmt.Errorf("dial pool: %w", err)
	}

	// CRITICAL: Do NOT store newConn in c.conn yet. If readLoop picks it up before
	// login completes, it will start reading from it, triggering the gorilla/websocket
	// panic "repeated read on failed websocket connection".
	// c.conn is only set after loginSuccess is confirmed below.

	// Drain login response channel to remove stale values
	select {
	case <-c.loginRespCh:
	default:
	}

	// Send login directly on newConn (NOT via sendRequest which uses c.conn).
	// This avoids exposing newConn to the concurrent readLoop goroutine.
	loginReq := map[string]interface{}{
		"method": "login",
		"params": map[string]string{
			"address": c.minerAddr,
		},
	}
	loginData, err := json.Marshal(loginReq)
	if err != nil {
		newConn.Close()
		return fmt.Errorf("marshal login: %w", err)
	}
	if err := newConn.WriteMessage(websocket.TextMessage, loginData); err != nil {
		newConn.Close()
		return fmt.Errorf("send login: %w", err)
	}
	c.log.Debugf("Login request sent (direct write)")

	// CRITICAL: Read WebSocket messages directly until loginSuccess is confirmed.
	// The readLoop is not running (initial connect) or the loop iteration
	// that called us will continue (reconnection), so we must process messages
	// here to avoid login timeout.
	c.log.Debugf("dialAndLogin: waiting for login response (reading WebSocket directly)...")
	loginDeadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(loginDeadline) {
		if err := newConn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
			c.log.Debugf("dialAndLogin: setReadDeadline error: %v", err)
		}

		_, message, err := newConn.ReadMessage()
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue // Timeout but still within login deadline
			}
			// Connection error — fail login
			newConn.Close()
			return fmt.Errorf("read login response: %w", err)
		}

		// Process the message
		var msg map[string]interface{}
		if err := json.Unmarshal(message, &msg); err != nil {
			c.log.Debugf("dialAndLogin: unmarshal error: %v", err)
			continue
		}

		method, _ := msg["method"].(string)
		switch method {
		case "loginSuccess":
			// Login confirmed — NOW store the connection in c.conn.
			// Replacing the old connection under lock ensures readLoop
			// picks up the confirmed connection atomically.
			c.mu.Lock()
			if c.conn != nil {
				c.conn.Close()
			}
			c.conn = newConn
			c.connected = true
			c.mu.Unlock()
			// Notify login response channel (for any concurrent waiters)
			select {
			case c.loginRespCh <- true:
			default:
			}
			c.log.Infof("Login confirmed (direct read)")
			return nil

		case "welcome":
			c.log.Infof("Received welcome from pool (direct read)")

		case "error":
			if params, ok := msg["params"].(map[string]interface{}); ok {
				if errMsg, ok := params["message"].(string); ok {
					c.log.Errorf("Pool login error: %s", errMsg)
				}
			}
			newConn.Close()
			return fmt.Errorf("login rejected by pool")
		}
	}

	// Login deadline expired
	newConn.Close()
	return fmt.Errorf("login timeout: no login response within 10s")
}

// login sends login request to pool
func (c *Client) login(ctx context.Context) error {
	req := map[string]interface{}{
		"method": "login",
		"params": map[string]string{
			"address": c.minerAddr,
		},
	}

	c.log.Debugf("Sending login (addr: %s...)", c.minerAddr[:min(16, len(c.minerAddr))])

	if err := c.sendRequest(ctx, req); err != nil {
		c.log.Errorf("Failed to send login request: %v", err)
		return err
	}

	c.log.Infof("Logged in with address: %s", c.minerAddr)
	return nil
}

// sendRequest sends a JSON-RPC request
func (c *Client) sendRequest(ctx context.Context, req map[string]interface{}) error {
	c.log.Infof(">>> sendRequest called")

	data, err := json.Marshal(req)
	if err != nil {
		c.log.Errorf("Marshal error: %v", err)
		return err
	}

	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()

	if conn == nil {
		c.log.Errorf("Connection is nil (sendRequest blocked)")
		return fmt.Errorf("not connected")
	}

	if len(data) > 120 {
		c.log.Debugf("Send: %s...", string(data[:120]))
	} else {
		c.log.Debugf("Send: %s", string(data))
	}

	// Use sendMu to prevent concurrent write to WebSocket (gorilla/websocket forbids concurrent write)
	// This also protects against concurrent writes from pingLoop
	c.sendMu.Lock()
	// Set write deadline to prevent indefinite block on broken connection
	conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	err = conn.WriteMessage(websocket.TextMessage, data)
	c.sendMu.Unlock()

	if err != nil {
		c.log.Errorf("WriteMessage error: %v", err)
		return err
	}

	c.log.Infof(">>> Message sent successfully")

	return nil
}

// readLoop is the persistent read loop with auto-reconnection.
// It runs in a single goroutine for the entire client lifetime.
// When the WebSocket connection drops, it automatically reconnects
// with exponential backoff and resumes reading messages.
// When Close() is called (e.g., pool switch), stopCh is closed and
// the loop exits permanently.
func (c *Client) readLoop(ctx context.Context) {
	c.log.Debugf("readLoop started (persistent, with auto-reconnection)")

	backoff := initialReconnectDelay

	for {
		// Check if context is cancelled
		select {
		case <-ctx.Done():
			c.log.Debugf("readLoop: context cancelled, exiting")
			return
		case <-c.stopCh:
			c.log.Debugf("readLoop: stopCh closed, exiting")
			return
		default:
		}

		c.mu.RLock()
		conn := c.conn
		c.mu.RUnlock()

		if conn == nil {
			// Connection is nil - attempt reconnection with backoff
			// But check stopCh and ctx during the wait
			c.log.Debugf("readLoop: conn is nil, reconnecting in %v...", backoff)

			select {
			case <-ctx.Done():
				c.log.Debugf("readLoop: context cancelled during backoff")
				return
			case <-c.stopCh:
				c.log.Debugf("readLoop: stopCh closed during backoff")
				return
			case <-time.After(backoff):
			}

			// Prevent concurrent dialAndLogin: if already dialing, wait for it to complete
			if atomic.LoadInt32(&c.dialing) == 1 {
				c.log.Debugf("readLoop: dialAndLogin already in progress, waiting...")
				<-c.dialDone
				continue // Re-check conn after waiting
			}

			if err := c.dialAndLogin(ctx); err != nil {
				c.log.Errorf("readLoop: reconnection failed: %v", err)
				backoff *= 2
				if backoff > maxReconnectDelay {
					backoff = maxReconnectDelay
				}
				continue
			}

			c.log.Infof("readLoop: reconnected successfully")
			backoff = initialReconnectDelay // Reset backoff on success
			continue
		}

		// Start ping goroutine for this connection
		pingStop := make(chan struct{})
		go c.pingLoop(conn, pingStop)
		defer close(pingStop) // Always stop pinger when this connection ends

		// Set read deadline to detect silent disconnections
		// (e.g., network partition, pool crash without TCP RST)
		if err := conn.SetReadDeadline(time.Now().Add(readDeadlineDuration)); err != nil {
			c.log.Debugf("readLoop: setReadDeadline error: %v", err)
		}

		_, message, err := conn.ReadMessage()
		if err != nil {
			c.log.Errorf("readLoop: read error: %v", err)
			c.mu.Lock()
			c.connected = false
			if c.conn != nil {
				c.conn.Close()
				c.conn = nil
			}
			c.mu.Unlock()
			// Loop back - will reconnect on next iteration with backoff
			continue
		}

		if len(message) > 120 {
		c.log.Debugf("Recv: %s...", string(message[:120]))
	} else {
		c.log.Debugf("Recv: %s", string(message))
	}
		c.handleMessage(ctx, message)
	}
}

// pingLoop sends periodic WebSocket ping frames to keep the connection alive.
// NAT gateways and firewalls typically drop idle TCP connections after 60-120s.
// Sending a ping every 25 seconds prevents this without the 90s readDeadline timeout.
func (c *Client) pingLoop(conn *websocket.Conn, stop <-chan struct{}) {
	ticker := time.NewTicker(25 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			if conn == nil {
				return
			}
			// Lock sendMu to prevent concurrent write with sendRequest
			// gorilla/websocket forbids concurrent WriteMessage calls
			c.sendMu.Lock()
			conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			err := conn.WriteMessage(websocket.PingMessage, nil)
			c.sendMu.Unlock()
			if err != nil {
				return // Connection lost, stop pinging (readLoop will reconnect)
			}
		}
	}
}

// handleMessage processes incoming message
func (c *Client) handleMessage(ctx context.Context, data []byte) {
	var msg map[string]interface{}
	if err := json.Unmarshal(data, &msg); err != nil {
		c.log.Errorf("Unmarshal error: %v", err)
		return
	}

	method, ok := msg["method"].(string)
	if !ok {
		return
	}

	switch method {
	case "welcome":
		c.log.Infof("Received welcome from pool")

	case "loginSuccess":
		c.log.Infof("Login successful")
		// Send login success to channel
		select {
		case c.loginRespCh <- true:
		default:
		}

	case "newJob":
		c.handleNewJob(msg)

	case "job":
		c.handleJob(msg)

	case "shareAccepted":
		// Pool sends {"method":"shareAccepted","params":{"jobId":...}}
		// Read jobId from params, not root message
		if params, ok := msg["params"].(map[string]interface{}); ok {
			jobID := getUint64FromMap(params, "jobId")
			c.resultCh <- &SubmitResult{
				Accepted: true,
				JobID:    jobID,
				Message:  "Share accepted",
			}
			c.log.Infof("Share accepted! jobId=%d", jobID)
		} else {
			c.resultCh <- &SubmitResult{
				Accepted: true,
				JobID:    0,
				Message:  "Share accepted",
			}
			c.log.Infof("Share accepted!")
		}

	case "shareRejected":
		if params, ok := msg["params"].(map[string]interface{}); ok {
			jobID := getUint64FromMap(params, "jobId")
			c.resultCh <- &SubmitResult{
				Accepted: false,
				JobID:    jobID,
				Message:  "Share rejected",
			}
			c.log.Warnf("Share rejected! jobId=%d", jobID)
		} else {
			c.resultCh <- &SubmitResult{
				Accepted: false,
				JobID:    0,
				Message:  "Share rejected",
			}
			c.log.Warnf("Share rejected")
		}

	case "jobExpired":
		// Pool sends {"method":"jobExpired","params":{"jobId":...,"reason":"..."}}
		// Treat as a rejection — the miner should get the latest job and try again
		var jobID uint64
		var reason string
		if params, ok := msg["params"].(map[string]interface{}); ok {
			jobID = getUint64FromMap(params, "jobId")
			if r, ok := params["reason"].(string); ok {
				reason = r
			}
		}
		c.log.Warnf("Job expired: jobId=%d, reason=%s", jobID, reason)
		c.resultCh <- &SubmitResult{
			Accepted: false,
			JobID:    jobID,
			Message:  "Job expired: " + reason,
		}

	case "error":
		if params, ok := msg["params"].(map[string]interface{}); ok {
			if errMsg, ok := params["message"].(string); ok {
				c.log.Errorf("Pool error: %s", errMsg)
				// Send rejection to result channel so miner can react
				c.resultCh <- &SubmitResult{
					Accepted: false,
					JobID:    getUint64FromMap(params, "jobId"),
					Message:  errMsg,
				}
				// Also handle login failure if still waiting
				select {
				case c.loginRespCh <- false:
				default:
				}
			}
		}
	}
}

// handleNewJob handles new job notification
func (c *Client) handleNewJob(msg map[string]interface{}) {
	params, ok := msg["params"].(map[string]interface{})
	if !ok {
		return
	}

	job := &MiningJob{
		JobID:        getUint64FromMap(params, "jobId"),
		Height:       getUint64FromMap(params, "height"),
		PrevHash:     getStringFromMap(params, "prevHash"),
		MerkleRoot:   getStringFromMap(params, "merkleRoot"),
		StateRoot:    getStringFromMap(params, "stateRoot"), // World State MPT root hash
		Difficulty:   getBigIntFromMap(params, "difficulty"),
		ExtraNonce:   getStringFromMap(params, "extraNonce"),
		Timestamp:    getInt64FromMap(params, "timestamp"),
		MinerAddress: getStringFromMap(params, "minerAddress"),
	}

	c.mu.Lock()
	c.lastJob = job
	c.mu.Unlock()

	select {
	case c.jobCh <- job:
		c.log.Infof("New job received: height=%d, jobId=%d", job.Height, job.JobID)
	default:
		c.log.Warnf("Job channel full, dropping job")
	}
}

// handleJob handles job response
func (c *Client) handleJob(msg map[string]interface{}) {
	c.handleNewJob(msg)
}

// GetJobChannel returns the job channel for receiving mining jobs
func (c *Client) GetJobChannel() <-chan *MiningJob {
	return c.jobCh
}

// GetResultChannel returns the result channel for receiving submission results
func (c *Client) GetResultChannel() <-chan *SubmitResult {
	return c.resultCh
}

// GetCurrentJob returns the current mining job
func (c *Client) GetCurrentJob() *MiningJob {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastJob
}

// SubmitShare submits a share to the pool
func (c *Client) SubmitShare(ctx context.Context, jobID, nonce uint64) error {
	req := map[string]interface{}{
		"method": "submit",
		"params": map[string]interface{}{
			"jobId": jobID,
			"nonce": nonce,
		},
	}

	c.log.Debugf("Submitting share: jobId=%d, nonce=%d", jobID, nonce)
	return c.sendRequest(ctx, req)
}

// IsConnected returns connection status
func (c *Client) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

// Close closes the connection and permanently stops the read loop.
// After Close, the client can be reused by calling Connect() again.
// readLoopStarted and stopCh are reset so that a subsequent Connect() call
// can start a fresh read loop (needed for pool switching).
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Close stopCh to signal readLoop to exit
	select {
	case <-c.stopCh:
		// Already closed
	default:
		close(c.stopCh)
	}

	// Close the current WebSocket connection
	if c.conn != nil {
		if err := c.conn.Close(); err != nil {
			c.log.Debugf("Close: connection close error: %v", err)
		}
		c.conn = nil
	}
	c.connected = false

	// Reset internal state so this Client can be reused by a subsequent Connect() call.
	// Without this, Connect() would see readLoopStarted=true and return nil immediately,
	// preventing reconnection after pool switch.
	c.readLoopStarted = false
	c.stopCh = make(chan struct{})

	return nil
}

// Helper functions
func getUint64FromMap(m map[string]interface{}, key string) uint64 {
	if v, ok := m[key]; ok {
		switch val := v.(type) {
		case float64:
			return uint64(val)
		case uint64:
			return val
		case int64:
			return uint64(val)
		case int:
			return uint64(val)
		}
	}
	return 0
}

func getStringFromMap(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func getBigIntFromMap(m map[string]interface{}, key string) *big.Int {
	if v, ok := m[key]; ok {
		switch val := v.(type) {
		case float64:
			return big.NewInt(int64(val))
		case uint64:
			return new(big.Int).SetUint64(val)
		case int64:
			return big.NewInt(val)
		case int:
			return big.NewInt(int64(val))
		case string:
			// Try parsing as string (for big.Int serialized as string)
			if bi, ok := new(big.Int).SetString(val, 10); ok {
				return bi
			}
		}
	}
	return big.NewInt(0)
}

func getInt64FromMap(m map[string]interface{}, key string) int64 {
	if v, ok := m[key]; ok {
		switch val := v.(type) {
		case float64:
			return int64(val)
		case int64:
			return val
		case int:
			return int64(val)
		}
	}
	return 0
}
