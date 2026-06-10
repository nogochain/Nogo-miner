// Package stratum provides TCP Stratum client for mining pool communication
package stratum

import (
	"bufio"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Reconnection backoff parameters
const (
	initialReconnectDelay = 2 * time.Second
	maxReconnectDelay     = 60 * time.Second
	readDeadlineDuration  = 120 * time.Second // Read deadline for silent disconnection detection
	writeDeadlineDuration = 5 * time.Second   // Short: broken writes fail fast, don't waste Worker time
	maxWriteFails         = 3                 // Consecutive write failures before forcing reconnect
	minConnectionLifetime = 10 * time.Second  // If connection lasts < this, treat as failed reconnect
	subscribeTimeout      = 10 * time.Second  // Timeout for mining.subscribe response
	authorizeTimeout      = 10 * time.Second  // Timeout for mining.authorize response
)

// Client represents a Stratum TCP client for pool communication
type Client struct {
	mu          sync.RWMutex
	sendMu      sync.Mutex // Protects TCP write operations from concurrent goroutines
	conn        net.Conn
	bufReader   *bufio.Reader
	poolURL     string
	poolHost    string
	poolPort    string
	minerAddr   string
	workerName  string
	password    string
	writeFails  int32 // atomic: consecutive write failures
	log         Logger
	connected   bool
	jobCh       chan *MiningJob
	resultCh    chan *SubmitResult
	lastJob     *MiningJob

	// Stratum protocol state
	subscribeID     string
	extraNonce1     string
	extraNonce2Size int
	nextRequestID   uint64 // atomic: incrementing JSON-RPC request ID

	// dialing prevents concurrent dialAndHandshake calls
	dialing     int32     // atomic flag: 1 = dialing in progress
	dialDone    chan struct{} // signaled when dialAndHandshake completes

	// readLoopStarted ensures the read loop goroutine is started exactly once.
	readLoopStarted bool

	// stopCh is closed when Close() is called to permanently stop the read loop.
	stopCh chan struct{}

	// readLoopDone is closed when readLoop truly exits.
	readLoopDone chan struct{}
}

// MiningJob represents a mining job from pool
type MiningJob struct {
	JobID        uint64   `json:"jobId"`
	Height       uint64   `json:"height"`
	PrevHash     string   `json:"prevHash"`
	MerkleRoot   string   `json:"merkleRoot"`
	StateRoot    string   `json:"stateRoot"`
	Difficulty   *big.Int `json:"difficulty"`
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

// Stratum JSON message types
type stratumRequest struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

type stratumResponse struct {
	ID     json.RawMessage `json:"id"`
	Result interface{}     `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type stratumNotification struct {
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

// NewClient creates a new Stratum TCP client
func NewClient(poolURL, minerAddr string, log Logger) *Client {
	if log == nil {
		log = &simpleLogger{}
	}

	// Parse pool URL to extract host and port
	poolHost := poolURL
	poolPort := "8008" // Default Stratum port

	// Handle ws://host:port or tcp://host:port or stratum+tcp://host:port or host:port formats
	rawURL := poolURL
	rawURL = strings.TrimPrefix(rawURL, "ws://")
	rawURL = strings.TrimPrefix(rawURL, "tcp://")
	rawURL = strings.TrimPrefix(rawURL, "stratum+tcp://")

	if parts := strings.SplitN(rawURL, ":", 2); len(parts) == 2 {
		poolHost = parts[0]
		poolPort = parts[1]
		if idx := strings.Index(poolPort, "/"); idx >= 0 {
			poolPort = poolPort[:idx]
		}
	}

	return &Client{
		poolURL:      poolURL,
		poolHost:     poolHost,
		poolPort:     poolPort,
		minerAddr:    minerAddr,
		workerName:   minerAddr,
		password:     "x",
		log:          log,
		jobCh:        make(chan *MiningJob, 10),
		resultCh:     make(chan *SubmitResult, 10),
		stopCh:       make(chan struct{}),
		readLoopDone: make(chan struct{}),
	}
}

// Connect establishes TCP connection to pool and performs Stratum handshake.
// On the first call, it also starts the persistent read loop that handles
// auto-reconnection when the connection drops. Subsequent calls are no-ops
// if the read loop is already running.
func (c *Client) Connect(ctx context.Context) error {
	c.mu.Lock()
	if c.readLoopStarted {
		c.mu.Unlock()
		c.log.Debugf("Connect called but read loop already running, skipping")
		return nil
	}
	c.readLoopStarted = true
	c.mu.Unlock()

	c.log.Infof("Connecting to pool: %s:%s", c.poolHost, c.poolPort)
	log.Printf("[NOGOMINER] Attempting TCP connection to pool: %s:%s", c.poolHost, c.poolPort)

	if err := c.dialAndHandshake(ctx); err != nil {
		c.log.Errorf("Initial connection failed: %v", err)
		log.Printf("[NOGOMINER] ❌ Connection FAILED: %v", err)
		return err
	}

	log.Printf("[NOGOMINER] ✅ Connection and handshake SUCCESSFUL!")

	go c.readLoop(context.Background())

	return nil
}

// dialAndHandshake dials TCP and performs Stratum subscribe + authorize.
// Used for both initial connect and reconnection.
// IMPORTANT: This function does NOT set c.conn until AFTER handshake is confirmed.
// It uses a local newConn variable throughout the handshake. This prevents a
// race condition where readLoop picks up the connection mid-handshake.
func (c *Client) dialAndHandshake(ctx context.Context) error {
	if !atomic.CompareAndSwapInt32(&c.dialing, 0, 1) {
		c.log.Debugf("dialAndHandshake: already dialing, waiting for completion")
		<-c.dialDone
		c.mu.RLock()
		conn := c.conn
		connected := c.connected
		c.mu.RUnlock()
		if conn != nil && connected {
			return nil
		}
		return fmt.Errorf("previous dial failed, please retry")
	}
	c.dialDone = make(chan struct{})
	defer func() {
		atomic.StoreInt32(&c.dialing, 0)
		close(c.dialDone)
	}()

	// Resolve TCP address
	tcpAddr, err := net.ResolveTCPAddr("tcp", c.poolHost+":"+c.poolPort)
	if err != nil {
		return fmt.Errorf("resolve address: %w", err)
	}

	// Dial with timeout
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	newConn, err := dialer.DialContext(ctx, "tcp", tcpAddr.String())
	if err != nil {
		return fmt.Errorf("dial tcp: %w", err)
	}

	// Enable TCP keepalive to detect dead connections faster
	if tcpConn, ok := newConn.(*net.TCPConn); ok {
		if err := tcpConn.SetKeepAlive(true); err != nil {
			c.log.Debugf("setKeepAlive: %v", err)
		}
		if err := tcpConn.SetKeepAlivePeriod(30 * time.Second); err != nil {
			c.log.Debugf("setKeepAlivePeriod: %v", err)
		}
	}

	// Set initial read deadline
	if err := newConn.SetReadDeadline(time.Now().Add(readDeadlineDuration)); err != nil {
		c.log.Debugf("setReadDeadline: %v", err)
	}

	// Create buffered reader for line-based reading
	bufReader := bufio.NewReader(newConn)

	// Step 1: mining.subscribe
	subID := atomic.AddUint64(&c.nextRequestID, 1)
	subscribeReq := map[string]interface{}{
		"id":     subID,
		"method": "mining.subscribe",
		"params": []string{"Nogominer/1.0.0"},
	}

	subscribeData, err := json.Marshal(subscribeReq)
	if err != nil {
		newConn.Close()
		return fmt.Errorf("marshal subscribe: %w", err)
	}

	if err := newConn.SetWriteDeadline(time.Now().Add(writeDeadlineDuration)); err != nil {
		c.log.Debugf("setWriteDeadline subscribe: %v", err)
	}
	if _, err := newConn.Write(append(subscribeData, '\n')); err != nil {
		newConn.Close()
		return fmt.Errorf("send subscribe: %w", err)
	}
	if err := newConn.SetWriteDeadline(time.Time{}); err != nil {
		c.log.Debugf("clear write deadline: %v", err)
	}

	c.log.Debugf("mining.subscribe sent (id=%d)", subID)

	// Read subscribe response
	subscribeResp, err := c.readLineWithTimeout(bufReader, subscribeTimeout)
	if err != nil {
		newConn.Close()
		return fmt.Errorf("read subscribe response: %w", err)
	}

	var subResp stratumResponse
	if err := json.Unmarshal([]byte(subscribeResp), &subResp); err != nil {
		newConn.Close()
		return fmt.Errorf("parse subscribe response: %w", err)
	}

	if subResp.Error != nil {
		newConn.Close()
		return fmt.Errorf("subscribe rejected: code=%d msg=%s", subResp.Error.Code, subResp.Error.Message)
	}

	// Parse subscribe result:
	// [ [ ["mining.notify", "subscription_id"], ...], extraNonce1, extraNonce2Size ]
	if resultArr, ok := subResp.Result.([]interface{}); ok && len(resultArr) >= 3 {
		c.extraNonce1, _ = resultArr[1].(string)
		if extraNonce2Size, ok := resultArr[2].(float64); ok {
			c.extraNonce2Size = int(extraNonce2Size)
		}
		// Extract subscribeID from the first element
		if firstArr, ok := resultArr[0].([]interface{}); ok && len(firstArr) > 0 {
			if subArr, ok := firstArr[0].([]interface{}); ok && len(subArr) >= 2 {
				c.subscribeID, _ = subArr[1].(string)
			}
		}
	}

	c.log.Infof("Subscribed: extraNonce1=%s, extraNonce2Size=%d", c.extraNonce1, c.extraNonce2Size)

	// Step 2: mining.authorize
	authID := atomic.AddUint64(&c.nextRequestID, 1)
	authorizeReq := map[string]interface{}{
		"id":     authID,
		"method": "mining.authorize",
		"params": []string{c.minerAddr, c.password},
	}

	authorizeData, err := json.Marshal(authorizeReq)
	if err != nil {
		newConn.Close()
		return fmt.Errorf("marshal authorize: %w", err)
	}

	if err := newConn.SetWriteDeadline(time.Now().Add(writeDeadlineDuration)); err != nil {
		c.log.Debugf("setWriteDeadline authorize: %v", err)
	}
	if _, err := newConn.Write(append(authorizeData, '\n')); err != nil {
		newConn.Close()
		return fmt.Errorf("send authorize: %w", err)
	}
	if err := newConn.SetWriteDeadline(time.Time{}); err != nil {
		c.log.Debugf("clear write deadline: %v", err)
	}

	c.log.Debugf("mining.authorize sent (id=%d, worker=%s)", authID, c.minerAddr)

	// Read authorize response
	authResp, err := c.readLineWithTimeout(bufReader, authorizeTimeout)
	if err != nil {
		newConn.Close()
		return fmt.Errorf("read authorize response: %w", err)
	}

	var authResult stratumResponse
	if err := json.Unmarshal([]byte(authResp), &authResult); err != nil {
		newConn.Close()
		return fmt.Errorf("parse authorize response: %w", err)
	}

	if authResult.Error != nil {
		newConn.Close()
		return fmt.Errorf("authorize rejected: code=%d msg=%s", authResult.Error.Code, authResult.Error.Message)
	}

	// Check if result is boolean true
	if authorized, ok := authResult.Result.(bool); ok && !authorized {
		newConn.Close()
		return fmt.Errorf("authorize failed: pool returned false")
	}

	c.log.Infof("Authorized: %s", c.minerAddr)

	// Handshake complete — store connection atomically
	c.mu.Lock()
	if c.conn != nil {
		c.conn.Close()
	}
	c.conn = newConn
	c.bufReader = bufReader
	c.connected = true
	c.mu.Unlock()

	return nil
}

// readLineWithTimeout reads a newline-delimited line with a specific timeout.
func (c *Client) readLineWithTimeout(reader *bufio.Reader, timeout time.Duration) (string, error) {
	type lineResult struct {
		line string
		err  error
	}

	resultCh := make(chan lineResult, 1)
	go func() {
		line, err := reader.ReadString('\n')
		resultCh <- lineResult{line, err}
	}()

	select {
	case res := <-resultCh:
		return strings.TrimSpace(res.line), res.err
	case <-time.After(timeout):
		return "", fmt.Errorf("read timeout after %v", timeout)
	}
}

// readLoop is the persistent read loop with auto-reconnection.
// It runs in a single goroutine for the entire client lifetime.
// When the TCP connection drops, it automatically reconnects
// with exponential backoff and resumes reading messages.
// When Close() is called, stopCh is closed and the loop exits permanently.
func (c *Client) readLoop(ctx context.Context) {
	defer func() {
		c.mu.Lock()
		if c.conn != nil {
			c.conn.Close()
			c.conn = nil
			c.bufReader = nil
		}
		c.connected = false
		c.mu.Unlock()
		close(c.readLoopDone)
		c.log.Debugf("readLoop: terminated")
	}()

	c.log.Debugf("readLoop started (persistent, with auto-reconnection)")

	backoff := initialReconnectDelay
	connStartTime := time.Now()

	for {
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
		reader := c.bufReader
		c.mu.RUnlock()

		if reader == nil {
			c.log.Debugf("readLoop: reader is nil, reconnecting in %v...", backoff)

			select {
			case <-ctx.Done():
				return
			case <-c.stopCh:
				return
			case <-time.After(backoff):
			}

			if atomic.LoadInt32(&c.dialing) == 1 {
				<-c.dialDone
				continue
			}

			if err := c.dialAndHandshake(ctx); err != nil {
				c.log.Errorf("readLoop: reconnection failed: %v", err)
				backoff *= 2
				if backoff > maxReconnectDelay {
					backoff = maxReconnectDelay
				}
				continue
			}

			c.log.Infof("readLoop: reconnected successfully")
			connStartTime = time.Now()
			backoff = initialReconnectDelay
			continue
		}

		// Refresh read deadline before each read
		c.mu.RLock()
		conn := c.conn
		c.mu.RUnlock()

		if conn != nil {
			if err := conn.SetReadDeadline(time.Now().Add(readDeadlineDuration)); err != nil {
				c.log.Debugf("readLoop: setReadDeadline error: %v", err)
			}
		}

		// Read next line
		line, err := reader.ReadString('\n')
		if err != nil {
			connLifetime := time.Since(connStartTime)
			c.log.Errorf("readLoop: read error after %v: %v", connLifetime.Round(time.Second), err)

			c.mu.Lock()
			c.connected = false
			if c.conn != nil {
				c.conn.Close()
				c.conn = nil
				c.bufReader = nil
			}
			c.mu.Unlock()

			if connLifetime < minConnectionLifetime && backoff < maxReconnectDelay {
				backoff *= 2
				if backoff > maxReconnectDelay {
					backoff = maxReconnectDelay
				}
			}
			continue
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Reset backoff on successful read
		if backoff > initialReconnectDelay {
			backoff = initialReconnectDelay
		}
		connStartTime = time.Now()

		c.handleMessage(ctx, []byte(line))
	}
}

// handleMessage processes incoming JSON message (notification or response)
func (c *Client) handleMessage(ctx context.Context, data []byte) {
	// Try to parse as Stratum notification (has "method" but no "id")
	var notif stratumNotification
	if err := json.Unmarshal(data, &notif); err == nil && notif.Method != "" {
		c.handleNotification(notif.Method, notif.Params)
		return
	}

	// Could be a response to a request (subscribe, authorize, submit)
	var resp stratumResponse
	if err := json.Unmarshal(data, &resp); err == nil && resp.ID != nil {
		c.handleSubmitResponse(&resp)
		return
	}
}

// handleNotification processes a Stratum notification
func (c *Client) handleNotification(method string, params json.RawMessage) {
	switch method {
	case "mining.notify":
		c.handleNotify(params)
	default:
		c.log.Debugf("Unknown notification: %s", method)
	}
}

// handleSubmitResponse processes a Stratum response (typically to mining.submit)
func (c *Client) handleSubmitResponse(resp *stratumResponse) {
	if resp.Error != nil {
		c.log.Errorf("Stratum error (code=%d): %s", resp.Error.Code, resp.Error.Message)
		c.resultCh <- &SubmitResult{
			Accepted: false,
			JobID:    0,
			Message:  fmt.Sprintf("Stratum error: %s", resp.Error.Message),
		}
		return
	}

	// mining.submit response: result is typically true/false
	if accepted, ok := resp.Result.(bool); ok {
		if accepted {
			c.resultCh <- &SubmitResult{
				Accepted: true,
				JobID:    0,
				Message:  "Share accepted",
			}
			c.log.Infof("Share accepted!")
		} else {
			c.resultCh <- &SubmitResult{
				Accepted: false,
				JobID:    0,
				Message:  "Share rejected by pool",
			}
			c.log.Warnf("Share rejected by pool")
		}
	}
}

// handleNotify processes a mining.notify notification
func (c *Client) handleNotify(params json.RawMessage) {
	var p []interface{}
	if err := json.Unmarshal(params, &p); err != nil || len(p) < 9 {
		c.log.Errorf("Invalid mining.notify params: %v", err)
		return
	}

	jobIDStr, _ := p[0].(string)
	prevHash, _ := p[1].(string)
	coinbase1, _ := p[2].(string)
	coinbase2, _ := p[3].(string)
	merkleBranches, _ := p[4].([]interface{})
	version, _ := p[5].(string)
	nBits, _ := p[6].(string)
	nTime, _ := p[7].(string)
	cleanJobs, _ := p[8].(bool)

	// Parse jobID string to uint64
	var jobID uint64
	if _, err := fmt.Sscanf(jobIDStr, "%d", &jobID); err != nil {
		c.log.Errorf("Parse jobID: %v", err)
		return
	}

	// Parse difficulty from nBits (big-endian hex target)
	difficulty := new(big.Int)
	difficulty.SetString(nBits, 10)
	if difficulty.Sign() <= 0 {
		difficulty = big.NewInt(1)
	}

	// Parse timestamp from nTime (hex string, big-endian 4 bytes)
	var timestamp int64
	if nTime != "" {
		tsBytes, err := hex.DecodeString(nTime)
		if err == nil && len(tsBytes) > 0 {
			for i := 0; i < len(tsBytes); i++ {
				timestamp = (timestamp << 8) | int64(tsBytes[i])
			}
		}
	}
	if timestamp == 0 {
		timestamp = time.Now().Unix()
	}

	_ = coinbase1
	_ = coinbase2
	_ = merkleBranches
	_ = version
	_ = cleanJobs

	// ── Parse NogoChain extensions (indices 9-13) ──
	// These fields are appended by NogoPool sendStratumJob and are NOT
	// part of the standard Stratum protocol. They provide stateRoot,
	// height, merkleRoot, extraNonce, and minerAddress for correct PoW.
	var stateRoot string
	var notifHeight uint64
	var merkleRootStr string
	var extraNonceFromJob string
	var minerAddress string

	if len(p) > 9 {
		stateRoot, _ = p[9].(string)
	}
	if len(p) > 10 {
		if heightStr, ok := p[10].(string); ok {
			fmt.Sscanf(heightStr, "%d", &notifHeight)
		}
	}
	if len(p) > 11 {
		merkleRootStr, _ = p[11].(string)
	}
	if len(p) > 12 {
		extraNonceFromJob, _ = p[12].(string)
	}
	if len(p) > 13 {
		minerAddress, _ = p[13].(string)
	}

	c.mu.RLock()
	extraNonce1 := c.extraNonce1
	c.mu.RUnlock()

	// Prefer job-provided extraNonce over subscription extraNonce1
	if extraNonceFromJob != "" {
		extraNonce1 = extraNonceFromJob
	}

	job := &MiningJob{
		JobID:        jobID,
		Height:       notifHeight,
		PrevHash:     prevHash,
		MerkleRoot:   merkleRootStr,
		StateRoot:    stateRoot,
		Difficulty:   difficulty,
		ExtraNonce:   extraNonce1,
		Timestamp:    timestamp,
		MinerAddress: minerAddress,
	}

	c.mu.Lock()
	c.lastJob = job
	c.mu.Unlock()

	select {
	case c.jobCh <- job:
		c.log.Infof("New job received: jobId=%d, diff=%s", job.JobID, job.Difficulty.String())
	default:
		c.log.Warnf("Job channel full, dropping job")
	}
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

// SendHashReport is a no-op for Stratum protocol.
// Stratum does not support explicit hash rate reports; hash rate is inferred
// from submitted shares by the pool.
func (c *Client) SendHashReport(ctx context.Context, hashes uint64) error {
	return nil
}

// SubmitShare submits a share via mining.submit
func (c *Client) SubmitShare(ctx context.Context, jobID, nonce uint64) error {
	// Generate extraNonce2 and nTime for Stratum submission
	extraNonce2 := fmt.Sprintf("%08x", time.Now().UnixNano()&0xFFFFFFFF)
	nTime := fmt.Sprintf("%08x", time.Now().Unix())
	nonceHex := fmt.Sprintf("%016x", nonce)
	jobIDStr := fmt.Sprintf("%d", jobID)

	reqID := atomic.AddUint64(&c.nextRequestID, 1)
	req := map[string]interface{}{
		"id":     reqID,
		"method": "mining.submit",
		"params": []string{
			c.workerName,
			jobIDStr,
			extraNonce2,
			nTime,
			nonceHex,
		},
	}

	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal submit: %w", err)
	}

	c.sendMu.Lock()
	defer c.sendMu.Unlock()

	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()

	if conn == nil {
		return fmt.Errorf("not connected")
	}

	if err := conn.SetWriteDeadline(time.Now().Add(writeDeadlineDuration)); err != nil {
		c.log.Debugf("setWriteDeadline submit: %v", err)
	}
	if _, err := conn.Write(append(data, '\n')); err != nil {
		if err := conn.SetWriteDeadline(time.Time{}); err != nil {
			c.log.Debugf("clear write deadline: %v", err)
		}
		fails := atomic.AddInt32(&c.writeFails, 1)
		if int(fails) >= maxWriteFails {
			conn.Close()
			atomic.StoreInt32(&c.writeFails, 0)
		}
		return fmt.Errorf("submit share: %w", err)
	}

	if err := conn.SetWriteDeadline(time.Time{}); err != nil {
		c.log.Debugf("clear write deadline: %v", err)
	}

	atomic.StoreInt32(&c.writeFails, 0)
	c.log.Debugf("Share submitted: jobId=%d, nonce=%d", jobID, nonce)
	return nil
}

// IsReconnecting returns true if a dialAndHandshake is currently in progress.
func (c *Client) IsReconnecting() bool {
	return atomic.LoadInt32(&c.dialing) == 1
}

// IsAlive returns true if the readLoop goroutine is running and can handle
// reconnection autonomously.
func (c *Client) IsAlive() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.readLoopStarted {
		return false
	}
	select {
	case <-c.stopCh:
		return false
	default:
		return true
	}
}

// IsConnected returns connection status
func (c *Client) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

// Close closes the connection and permanently stops the read loop.
// After Close, the client can be reused by calling Connect() again.
func (c *Client) Close() error {
	c.mu.Lock()

	select {
	case <-c.stopCh:
	default:
		close(c.stopCh)
	}

	if c.conn != nil {
		if err := c.conn.Close(); err != nil {
			c.log.Debugf("Close: connection close error: %v", err)
		}
		c.conn = nil
		c.bufReader = nil
	}
	c.connected = false

	wasStarted := c.readLoopStarted
	done := c.readLoopDone
	c.mu.Unlock()

	if wasStarted {
		<-done
	}

	c.mu.Lock()
	c.readLoopStarted = false
	c.stopCh = make(chan struct{})
	c.readLoopDone = make(chan struct{})
	c.mu.Unlock()

	return nil
}