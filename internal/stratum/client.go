// Package stratum provides WebSocket Stratum client for mining pool communication
package stratum

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Client represents a Stratum client for pool communication
type Client struct {
	mu           sync.RWMutex
	conn         *websocket.Conn
	poolURL      string
	minerAddr    string
	log          Logger
	connected    bool
	jobCh        chan *MiningJob
	resultCh     chan *SubmitResult
	lastJob      *MiningJob
	subscribeID  string
	loginRespCh  chan bool  // Channel to wait for login response
}

// MiningJob represents a mining job from pool
type MiningJob struct {
	JobID        uint64   `json:"jobId"`
	Height       uint64   `json:"height"`
	PrevHash     string   `json:"prevHash"`
	MerkleRoot   string   `json:"merkleRoot"`
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
	}
}

// Connect establishes WebSocket connection to pool
func (c *Client) Connect(ctx context.Context) error {
	c.log.Infof("Connecting to pool: %s", c.poolURL)

	// Parse poolURL to get host
	parsedURL, err := url.Parse(c.poolURL)
	if err != nil {
		c.log.Errorf("Failed to parse pool URL: %v", err)
		return fmt.Errorf("parse pool URL: %w", err)
	}
	
	// Extract host:port from URL (strip protocol and path)
	poolHost := parsedURL.Host
	if poolHost == "" {
		// Fallback for URLs without explicit host
		poolHost = strings.TrimPrefix(parsedURL.Path, "//")
		if idx := strings.Index(poolHost, "/"); idx != -1 {
			poolHost = poolHost[:idx]
		}
	}
	
	c.log.Infof("Parsed pool URL: %s -> host: %s", c.poolURL, poolHost)

	// First test TCP connectivity
	c.log.Infof("Testing TCP connectivity to %s...", poolHost)
	tcpConn, err := net.DialTimeout("tcp", poolHost, 5*time.Second)
	if err != nil {
		c.log.Errorf("TCP connection failed: %v", err)
		return fmt.Errorf("tcp connect: %w", err)
	}
	tcpConn.Close()
	c.log.Infof("TCP connection successful")

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
		Proxy:            nil, // Don't use proxy
	}

	conn, _, err := dialer.Dial(c.poolURL, nil)
	if err != nil {
		c.log.Errorf("WebSocket dial failed: %v", err)
		return fmt.Errorf("dial pool: %w", err)
	}

	c.mu.Lock()
	c.conn = conn
	c.connected = true
	c.mu.Unlock()
	
	c.log.Infof("Connected to pool")

	// Start message reader FIRST before sending login
	c.log.Infof("Starting readMessages goroutine...")
	go c.readMessages(ctx)

	// Wait a bit for welcome message
	time.Sleep(100 * time.Millisecond)
	c.log.Infof("Sending login request...")

	// Login to pool
	if err := c.login(ctx); err != nil {
		c.log.Errorf("Login failed: %v", err)
		conn.Close()
		c.mu.Lock()
		c.connected = false
		c.mu.Unlock()
		return fmt.Errorf("login: %w", err)
	}

	c.log.Infof("Waiting for login response...")

	// Wait for login response (with timeout)
	select {
	case success := <-c.loginRespCh:
		if !success {
			c.log.Errorf("Login response: failed")
			return fmt.Errorf("login failed")
		}
		c.log.Infof("Login confirmed")
	case <-time.After(5 * time.Second):
		c.log.Errorf("Login timeout after 5 seconds")
		return fmt.Errorf("login timeout")
	case <-ctx.Done():
		c.log.Errorf("Context cancelled: %v", ctx.Err())
		return ctx.Err()
	}

	return nil
}

// login sends login request to pool
func (c *Client) login(ctx context.Context) error {
	req := map[string]interface{}{
		"method": "login",
		"params": map[string]string{
			"address": c.minerAddr,
		},
	}

	c.log.Debugf("Sending login request with address: %s", c.minerAddr)
	
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
		c.log.Errorf("Connection is nil")
		return fmt.Errorf("not connected")
	}

	c.log.Debugf("Sending message: %s", string(data))
	
	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		c.log.Errorf("WriteMessage error: %v", err)
		return err
	}
	
	c.log.Infof(">>> Message sent successfully")
	
	return nil
}

// readMessages reads incoming messages from pool
func (c *Client) readMessages(ctx context.Context) {
	c.log.Debugf("readMessages started")
	for {
		c.mu.RLock()
		conn := c.conn
		c.mu.RUnlock()

		if conn == nil {
			c.log.Debugf("readMessages: conn is nil, exiting")
			return
		}

		_, message, err := conn.ReadMessage()
		if err != nil {
			c.log.Errorf("Read error: %v", err)
			c.connected = false
			return
		}

		c.log.Debugf("Received message: %s", string(message))
		c.handleMessage(ctx, message)
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
		c.resultCh <- &SubmitResult{
			Accepted: true,
			JobID:    getUint64FromMap(msg, "jobId"),
			Message:  "Share accepted",
		}
		c.log.Infof("Share accepted!")
		
	case "shareRejected":
		c.resultCh <- &SubmitResult{
			Accepted: false,
			JobID:    getUint64FromMap(msg, "jobId"),
			Message:  "Share rejected",
		}
		c.log.Warnf("Share rejected")
		
	case "error":
		if params, ok := msg["params"].(map[string]interface{}); ok {
			if errMsg, ok := params["message"].(string); ok {
				c.log.Errorf("Pool error: %s", errMsg)
				// Send login failure to channel if still waiting
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

// Close closes the connection
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		if err := c.conn.Close(); err != nil {
			return err
		}
		c.conn = nil
	}
	c.connected = false
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
