// Package rpc provides RPC client for NogoChain node communication
package rpc

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"sync"
	"time"

	"github.com/nogochain/nogo-miner/internal/config"
	"github.com/nogochain/nogo-miner/internal/logger"
)

// Client represents an RPC client for node communication
type Client struct {
	mu           sync.RWMutex
	httpClient   *http.Client
	baseURL      string
	wsURL        string
	timeout      time.Duration
	maxRetries   int
	retryDelay   time.Duration
	log          *logger.Logger
	connected    bool
	lastConnect  time.Time
}

// BlockTemplate represents a block template for mining
type BlockTemplate struct {
	Height         uint64   `json:"height"`
	PrevHash       string   `json:"prevHash"`
	MerkleRoot     string   `json:"merkleRoot"`
	Timestamp      int64    `json:"timestamp"`
	DifficultyBits *big.Int `json:"difficultyBits"` // Changed to *big.Int
	MinerAddress   string   `json:"minerAddress"`
	Transactions   []Transaction `json:"transactions"`
	CoinbaseTx     *Transaction  `json:"coinbaseTx"`
	Target         string   `json:"target"`
	ChainID        uint64   `json:"chainId"`
}

// Transaction represents a blockchain transaction
type Transaction struct {
	Type      string `json:"type"`
	ChainID   uint64 `json:"chainId"`
	FromPubKey []byte `json:"fromPubKey,omitempty"` // Changed to []byte
	ToAddress string `json:"toAddress"`
	Amount    uint64 `json:"amount"`
	Fee       uint64 `json:"fee"`
	Nonce     uint64 `json:"nonce"`
	Data      string `json:"data,omitempty"`
	Signature []byte `json:"signature,omitempty"` // Changed to []byte
}

// SubmitWorkRequest represents a work submission request
type SubmitWorkRequest struct {
	Height     uint64 `json:"height"`
	Nonce      uint64 `json:"nonce"`
	BlockHash  string `json:"blockHash"`
	PrevHash   string `json:"prevHash"`
	MerkleRoot string `json:"merkleRoot"`
	Timestamp  int64  `json:"timestamp"`
}

// SubmitWorkResponse represents a work submission response
type SubmitWorkResponse struct {
	Accepted bool   `json:"accepted"`
	Message  string `json:"message,omitempty"`
	Error    string `json:"error,omitempty"`
}

// NodeInfo represents node information
type NodeInfo struct {
	Version     string `json:"version"`
	ChainID     uint64 `json:"chainId"`
	Height      uint64 `json:"height"`
	PeersCount  int    `json:"peersCount"`
	Syncing     bool   `json:"syncing"`
	NetworkID   string `json:"networkId"`
	ProtocolVer int    `json:"protocolVersion"`
}

// ErrorResponse represents an API error response
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
	Code    int    `json:"code"`
}

// NewClient creates a new RPC client
func NewClient(cfg *config.RPCConfig, log *logger.Logger) *Client {
	if log == nil {
		log, _ = logger.New(logger.Config{
			Level: "info",
		})
	}

	client := &Client{
		httpClient: &http.Client{
			Timeout: cfg.GetTimeout(),
		},
		baseURL:    cfg.URL,
		wsURL:      cfg.WSURL,
		timeout:    cfg.GetTimeout(),
		maxRetries: cfg.MaxRetries,
		retryDelay: cfg.GetRetryDelay(),
		log:        log,
		connected:  false,
	}

	return client
}

// GetBlockTemplate fetches a block template for mining
func (c *Client) GetBlockTemplate(ctx context.Context) (*BlockTemplate, error) {
	url := c.baseURL + "/block/template"
	
	var template BlockTemplate
	err := c.doRequest(ctx, http.MethodGet, url, nil, &template)
	if err != nil {
		return nil, fmt.Errorf("get block template: %w", err)
	}

	c.log.Infof("Fetched block template: height=%d, prevHash=%s, difficulty=%d",
		template.Height, template.PrevHash[:16], template.DifficultyBits)

	return &template, nil
}

// SubmitWork submits a solution to the node
func (c *Client) SubmitWork(ctx context.Context, req SubmitWorkRequest) (*SubmitWorkResponse, error) {
	url := c.baseURL + "/mining/submit"
	
	var resp SubmitWorkResponse
	err := c.doRequest(ctx, http.MethodPost, url, req, &resp)
	if err != nil {
		return nil, fmt.Errorf("submit work: %w", err)
	}

	if resp.Accepted {
		c.log.Infof("Work accepted: height=%d, nonce=%d", req.Height, req.Nonce)
	} else {
		c.log.Warnf("Work rejected: height=%d, nonce=%d, reason=%s",
			req.Height, req.Nonce, resp.Error)
	}

	return &resp, nil
}

// GetNodeInfo fetches node information
func (c *Client) GetNodeInfo(ctx context.Context) (*NodeInfo, error) {
	url := c.baseURL + "/chain/info"
	
	var info NodeInfo
	err := c.doRequest(ctx, http.MethodGet, url, nil, &info)
	if err != nil {
		return nil, fmt.Errorf("get node info: %w", err)
	}

	c.log.Infof("Connected to node: version=%s, height=%d, peers=%d",
		info.Version, info.Height, info.PeersCount)

	return &info, nil
}

// HealthCheck checks if the node is healthy
func (c *Client) HealthCheck(ctx context.Context) error {
	url := c.baseURL + "/health"
	
	var result map[string]string
	err := c.doRequest(ctx, http.MethodGet, url, nil, &result)
	if err != nil {
		return fmt.Errorf("health check: %w", err)
	}

	if result["status"] != "ok" {
		return fmt.Errorf("node unhealthy: status=%s", result["status"])
	}

	return nil
}

// doRequest performs an HTTP request with retries
func (c *Client) doRequest(ctx context.Context, method, url string, body, result interface{}) error {
	var lastErr error

	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			c.log.Debugf("Retry %d/%d for %s", attempt, c.maxRetries, url)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(c.retryDelay):
			}
		}

		err := c.doRequestOnce(ctx, method, url, body, result)
		if err == nil {
			if !c.connected {
				c.connected = true
				c.lastConnect = time.Now()
				c.log.Info("Connected to RPC server")
			}
			return nil
		}

		lastErr = err
		c.log.Debugf("Request failed: %v", err)

		// Don't retry on context cancellation or certain errors
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}

	c.connected = false
	return fmt.Errorf("max retries exceeded: %w", lastErr)
}

// doRequestOnce performs a single HTTP request
func (c *Client) doRequestOnce(ctx context.Context, method, url string, body, result interface{}) error {
	var req *http.Request
	var err error

	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal body: %w", err)
		}
		req, err = http.NewRequestWithContext(ctx, method, url, bytes.NewReader(jsonBody))
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
	} else {
		req, err = http.NewRequestWithContext(ctx, method, url, nil)
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp ErrorResponse
		if err := json.Unmarshal(respBody, &errResp); err == nil {
			return fmt.Errorf("HTTP %d: %s - %s", resp.StatusCode, errResp.Error, errResp.Message)
		}
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	if err := json.Unmarshal(respBody, result); err != nil {
		return fmt.Errorf("unmarshal response: %w", err)
	}

	return nil
}

// IsConnected returns connection status
func (c *Client) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

// GetBaseURL returns the base URL
func (c *Client) GetBaseURL() string {
	return c.baseURL
}

// GetWSURL returns the WebSocket URL
func (c *Client) GetWSURL() string {
	return c.wsURL
}

// SetTimeout sets the request timeout
func (c *Client) SetTimeout(timeout time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.timeout = timeout
	c.httpClient.Timeout = timeout
}

// Close closes the client
func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.connected = false
	c.httpClient.CloseIdleConnections()
	c.log.Info("RPC client closed")
}

// BytesToHex converts bytes to hex string
func BytesToHex(data []byte) string {
	return hex.EncodeToString(data)
}

// HexToBytes converts hex string to bytes
func HexToBytes(s string) ([]byte, error) {
	return hex.DecodeString(s)
}
