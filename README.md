# Nogo-miner — NogoChain Mining Software

[![License](https://img.shields.io/badge/license-LGPLv3-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/go-1.21.5-00ADD8.svg)](https://golang.org)
[![Build Status](https://github.com/nogochain/nogo-miner/workflows/CI/badge.svg)](https://github.com/nogochain/nogo-miner/actions)

**High-performance mining software for NogoChain blockchain with full NogoPow consensus algorithm support**

---

## Table of Contents

- [Overview](#overview)
- [Features](#features)
- [System Requirements](#system-requirements)
- [Installation](#installation)
- [Quick Start](#quick-start)
- [Configuration](#configuration)
- [Usage](#usage)
- [Mining Pool Integration](#mining-pool-integration)
- [NogoPow Algorithm](#nogopow-algorithm)
- [Monitoring & Metrics](#monitoring--metrics)
- [Performance Optimization](#performance-optimization)
- [Troubleshooting](#troubleshooting)
- [Security Best Practices](#security-best-practices)
- [Development](#development)
- [API Reference](#api-reference)
- [Contributing](#contributing)
- [License](#license)

---

## Overview

Nogo-miner is the official mining software for NogoChain, implementing the complete NogoPow proof-of-work algorithm. It provides high-performance parallel computing capabilities while maintaining exact consistency with NogoChain node implementations.

### Architecture

```
┌─────────────────────────────────────────────────────────┐
│                     NogoMiner v1.0.0                     │
├─────────────────────────────────────────────────────────┤
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐     │
│  │   Config    │  │   Logger    │  │   Monitor   │     │
│  │   Manager   │  │   System    │  │   System    │     │
│  └─────────────┘  └─────────────┘  └─────────────┘     │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐     │
│  │  RPC Client │  │ Pool Manager│  │  Stratum    │     │
│  │  (HTTP/WS)  │  │ (Failover)  │  │  Client     │     │
│  └─────────────┘  └─────────────┘  └─────────────┘     │
│  ┌─────────────────────────────────────────────────┐   │
│  │         NogoPow Engine                          │   │
│  │  - Seed Cache  - Parallel Computation (4-way)   │   │
│  │  - Difficulty Adj - Share Validation            │   │
│  │  - XOR Mixing   - Matrix Operations             │   │
│  └─────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────┘
                          │
                          │ WebSocket / Stratum Protocol
                          ▼
              ┌───────────────────────┐
              │    NogoPool Server    │
              └───────────────────────┘
```

---

## Features

### Core Features

- **Full NogoPow Algorithm**: Exact implementation matching NogoChain nodes
  - Seed-based cache computation
  - XOR mixing with header hash
  - Dynamic difficulty verification
  - Share-level PoW validation

- **High-Performance Mining**
  - Multi-core CPU parallel computing (configurable threads)
  - Optimized matrix operations
  - Memory pool for object reuse
  - Configurable batch size

- **Mining Pool Support**
  - Stratum protocol over WebSocket
  - Multiple pool failover with priority support
  - Automatic reconnection with exponential backoff (2s-60s)
  - Health check loop with reconnection detection
  - Share difficulty acceptance

- **Production-Ready**
  - Race-condition-free connection lifecycle (readLoopDone synchronization)
  - Duplicate readLoop goroutine prevention (IsReconnecting guard)
  - Comprehensive error handling
  - Resource management with cleanup
  - Concurrency safety (c.mu/sendMu layered locking)

- **Monitoring & Metrics**
  - Real-time hashrate display (5s refresh)
  - Accepted/rejected/invalid share tracking
  - Miner uptime and worker status display
  - Detailed logging with rotation

---

## System Requirements

### Minimum Requirements

- **OS**: Windows 10+, Linux (Ubuntu 18.04+), macOS 10.15+
- **CPU**: Dual-core processor (2+ cores)
- **RAM**: 2 GB
- **Network**: Stable internet connection (1 Mbps+)
- **Storage**: 100 MB for application + logs

### Recommended Requirements

- **OS**: Linux (Ubuntu 20.04+), Windows 11+
- **CPU**: Quad-core processor (4+ cores), e.g., Intel i5 / AMD Ryzen 5
- **RAM**: 4 GB or more
- **Network**: Broadband connection (10 Mbps+)
- **Storage**: SSD for better performance

### Build Requirements (if compiling from source)

- **Go**: 1.21.5 or later
- **Git**: For repository cloning
- **Build Tools**: GCC (for CGO if needed)

---

## Installation

### Method 1: Download Pre-compiled Binaries (Recommended)

Download the latest release from [GitHub Releases](https://github.com/nogochain/nogo-miner/releases):

```bash
# Linux (AMD64)
wget https://github.com/nogochain/nogo-miner/releases/download/v1.0.0/nogominer-linux-amd64
chmod +x nogominer-linux-amd64
sudo mv nogominer-linux-amd64 /usr/local/bin/nogominer

# Windows (AMD64)
# Download nogominer-windows-amd64.exe and rename to nogominer.exe

# macOS (AMD64)
wget https://github.com/nogochain/nogo-miner/releases/download/v1.0.0/nogominer-macos-amd64
chmod +x nogominer-macos-amd64
sudo mv nogominer-macos-amd64 /usr/local/bin/nogominer
```

### Method 2: Build from Source

```bash
# Clone repository
git clone https://github.com/nogochain/nogo-miner.git
cd nogo-miner

# Build (production)
go build -ldflags="-s -w" -o nogominer ./cmd/nogominer

# Build with race detection (debugging)
go build -race -o nogominer ./cmd/nogominer

# Install to GOPATH/bin
go install ./cmd/nogominer
```

### Method 3: Docker (Coming Soon)

```bash
# Pull image
docker pull nogochain/nogo-miner:latest

# Run with config volume
docker run -d --name nogominer \
  -v /path/to/config:/config \
  -v /path/to/logs:/logs \
  --restart unless-stopped \
  nogochain/nogo-miner:latest \
  -config /config/config.json
```

---

## Quick Start

### Step 1: Generate Mining Address

First, you need a valid NogoChain address to receive mining rewards:

```bash
# Using NogoChain node CLI
nogo-cli wallet generate

# Output:
# Address: NOGO00d20c827391ea4e9df242418a33ba4c47bcfe92bf1aa2a8d09df72b72623b7f52cb2200e6
# Private Key: <save_this_securely>
```

### Step 2: Create Configuration

```bash
# Copy example config
cp configs/config.example.json config.json

# Edit config.json with your mining address
# Replace "address" with your NOGO address from Step 1
```

### Step 3: Start Mining

```bash
# Linux/macOS
./nogominer

# Windows
nogominer.exe
```

### Expected Output

```
2026-05-19 10:00:00 🚀 NogoMiner v1.0.0 starting
2026-05-19 10:00:01 ✅ Connected to pool: ws://localhost:1819/stratum
2026-05-19 10:00:01 ✅ Authentication successful
2026-05-19 10:00:02 ⛏️  Mining started with 4 threads
2026-05-19 10:00:12 📊 Hashrate: 1.25 KH/s | Accepted: 12 | Rejected: 0
2026-05-19 10:00:22 📊 Hashrate: 1.30 KH/s | Accepted: 25 | Rejected: 0
```

---

## Configuration

### Configuration File Structure

The configuration file (`config.json`) uses JSON format with the following sections:

```json
{
  "rpc": { ... },           // RPC connection settings
  "pools": [ ... ],         // Mining pool configurations
  "miner": { ... },         // Mining parameters
  "logging": { ... },       // Logging configuration
  "monitor": { ... }        // Monitoring settings
}
```

### Complete Configuration Example

```json
{
  "rpc": {
    "url": "http://localhost:1818",
    "ws_url": "ws://localhost:1819/stratum",
    "timeout_seconds": 30,
    "max_retries": 5,
    "retry_delay_seconds": 2
  },
  "pools": [
    {
      "name": "NogoPool",
      "url": "http://localhost:1818",
      "ws_url": "ws://localhost:1819/stratum",
      "address": "NOGO00d20c827391ea4e9df242418a33ba4c47bcfe92bf1aa2a8d09df72b72623b7f52cb2200e6",
      "priority": 1,
      "enabled": true
    }
  ],
  "miner": {
    "threads": 4,
    "batch_size": 100,
    "share_difficulty": 0
  },
  "logging": {
    "level": "info",
    "file": "nogominer.log",
    "max_size_mb": 10,
    "max_backups": 3,
    "max_age_days": 30,
    "compress": false,
    "json_format": false
  },
  "monitor": {
    "enabled": true,
    "update_interval_seconds": 5,
    "prometheus_enabled": false,
    "prometheus_port": 9090
  }
}
```

### Configuration Options Reference

#### RPC Configuration

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `url` | string | `http://localhost:1818` | HTTP RPC endpoint URL |
| `ws_url` | string | `ws://localhost:1819/stratum` | WebSocket URL for real-time updates |
| `timeout_seconds` | int | `30` | Request timeout in seconds |
| `max_retries` | int | `5` | Maximum retry attempts |
| `retry_delay_seconds` | int | `2` | Delay between retries |

#### Pool Configuration (Array)

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `name` | string | Required | Pool identifier name |
| `url` | string | Required | Pool HTTP RPC URL |
| `ws_url` | string | Required | Pool WebSocket URL |
| `address` | string | Required | Your mining reward address (NOGO format) |
| `priority` | int | `1` | Pool priority (1=highest) |
| `enabled` | bool | `true` | Enable this pool |

#### Miner Configuration

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `threads` | int | `0` (auto) | Number of mining threads (0=auto-detect CPU cores) |
| `batch_size` | int | `100` | Batch size for share submission |
| `share_difficulty` | int | `0` (pool-provided) | Minimum share difficulty (0=use pool difficulty) |

#### Logging Configuration

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `level` | string | `info` | Log level: `debug`, `info`, `warn`, `error` |
| `file` | string | `nogominer.log` | Log file path |
| `max_size_mb` | int | `10` | Maximum log file size before rotation |
| `max_backups` | int | `3` | Maximum number of backup log files |
| `max_age_days` | int | `30` | Maximum log file age in days |
| `compress` | bool | `false` | Compress rotated logs |
| `json_format` | bool | `false` | Use JSON format for logs |

#### Monitor Configuration

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `enabled` | bool | `true` | Enable monitoring system |
| `update_interval_seconds` | int | `5` | Statistics update interval |
| `prometheus_enabled` | bool | `false` | Enable Prometheus metrics export |
| `prometheus_port` | int | `9090` | Prometheus metrics port |

### Multi-Pool Configuration (Failover)

Configure multiple pools for high availability:

```json
{
  "pools": [
    {
      "name": "Primary Pool",
      "url": "http://pool1.nogochain.io:1818",
      "ws_url": "ws://pool1.nogochain.io:1819/stratum",
      "address": "NOGO<your_address>",
      "priority": 1,
      "enabled": true
    },
    {
      "name": "Backup Pool",
      "url": "http://pool2.nogochain.io:1818",
      "ws_url": "ws://pool2.nogochain.io:1819/stratum",
      "address": "NOGO<your_address>",
      "priority": 2,
      "enabled": true
    }
  ]
}
```

The miner will automatically failover to the next pool if the primary becomes unavailable.

### Command-Line Override

Command-line arguments take precedence over config file:

```bash
# Override RPC URL
./nogominer -rpc-url http://custom-node:1818

# Override thread count
./nogominer -threads 8

# Override mining address
./nogominer -address NOGO<your_address>

# Specify custom config file
./nogominer -config /path/to/custom-config.json
```

### Environment Variables

```bash
# Set RPC endpoint
export NOGOMINER_RPC_URL="http://localhost:1818"

# Set WebSocket endpoint
export NOGOMINER_WS_URL="ws://localhost:1819/stratum"

# Set mining address
export NOGOMINER_ADDRESS="NOGO<your_address>"

# Set thread count
export NOGOMINER_THREADS="4"

# Set log level
export NOGOMINER_LOG_LEVEL="debug"

# Start miner
./nogominer
```

---

## Usage

### Basic Commands

```bash
# Start with default config
./nogominer

# Start with custom config
./nogominer -config /path/to/config.json

# Show version
./nogominer -version

# Show help
./nogominer -h
```

### Command-Line Flags

```
  -config string
        Config file path (default "config.json")
  -rpc-url string
        RPC server URL (overrides config)
  -ws-url string
        WebSocket URL (overrides config)
  -address string
        Mining address (overrides config)
  -threads int
        Number of mining threads, 0=auto-detect (default: 0)
  -log-level string
        Log level: debug, info, warn, error (default: "info")
  -version
        Display version information and exit
  -h    Display help information
```

### Running as a Service

#### Linux (Systemd)

Create `/etc/systemd/system/nogominer.service`:

```ini
[Unit]
Description=NogoChain Miner
After=network.target

[Service]
Type=simple
User=miner
WorkingDirectory=/opt/nogominer
ExecStart=/opt/nogominer/nogominer -config /opt/nogominer/config.json
Restart=on-failure
RestartSec=10

[Install]
WantedBy=multi-user.target
```

```bash
# Enable and start
sudo systemctl enable nogominer
sudo systemctl start nogominer
sudo systemctl status nogominer

# View logs
journalctl -u nogominer -f
```

#### Windows (Task Scheduler)

```batch
REM Create scheduled task
schtasks /create /tn "NogoMiner" /tr "C:\path\to\nogominer.exe" /sc onstart /ru SYSTEM
```

---

## Mining Pool Integration

### Supported Pools

Nogo-miner supports any mining pool implementing the standard Stratum protocol over WebSocket:

- ✅ **NogoPool** (official pool software)
- ✅ Custom pools with WebSocket Stratum support
- ✅ Multi-pool configurations with automatic failover

### Connection Workflow

```
1. Miner connects to pool via WebSocket
2. Authentication with mining address
3. Pool distributes mining jobs
4. Miner computes PoW using NogoPow
5. Valid shares submitted to pool
6. Pool validates and rewards miner
```

### Pool Configuration Example

```json
{
  "pools": [
    {
      "name": "NogoPool",
      "url": "http://localhost:1818",
      "ws_url": "ws://localhost:1819/stratum",
      "address": "NOGO00d20c827391ea4e9df242418a33ba4c47bcfe92bf1aa2a8d09df72b72623b7f52cb2200e6",
      "priority": 1,
      "enabled": true
    }
  ]
}
```

### Share Difficulty

The pool can assign difficulty dynamically:

```json
{
  "miner": {
    "share_difficulty": 0  // 0 = use pool-assigned difficulty
  }
}
```

For fixed difficulty (advanced):

```json
{
  "miner": {
    "share_difficulty": 100  // Fixed difficulty
  }
}
```

### Monitoring Pool Connection

```bash
# Check pool connection status
curl http://localhost:1818/api/miners

# View pool statistics
curl http://localhost:1818/api/stats
```

---

## NogoPow Algorithm

### Algorithm Overview

NogoPow is NogoChain's proof-of-work algorithm, featuring seed-based cache computation with XOR mixing:

```
Block Header → HeaderHash → Seed Cache Generation → XOR Mixing → PoW Hash → Difficulty Check
```

### Key Parameters

| Parameter | Value | Description |
|-----------|-------|-------------|
| Seed Size | Configurable | Seed bytes for cache generation |
| XOR Rounds | 8 rounds | Mixing iterations |
| Concurrent Workers | Configurable (default: 4) | Parallel computation threads |
| Write Deadline | 5 seconds | Fast failure for broken connections |
| Read Deadline | 120 seconds | Silent disconnection detection |

### Core Implementation

```go
// Engine.Mine performs the core mining loop
func (e *Engine) Mine(headerHash []byte, seed []byte, difficulty uint64, 
    nonceStart uint64, nonceEnd uint64, resultCh chan *MiningResult)

// Engine.VerifyPoW verifies a computed PoW solution
func (e *Engine) VerifyPoW(headerHash []byte, seed []byte, 
    nonce uint64, difficulty uint64) bool
```

### Algorithm Consistency

Nogo-miner maintains exact consistency with NogoChain nodes:

- ✅ Seed cache computation: Identical to node implementation
- ✅ XOR mixing operations: 8 rounds, matching consensus
- ✅ Difficulty target calculation: Dynamic, node-compatible
- ✅ Share validation: Full PoW verification
- ✅ Header hash computation: Same as consensus layer

### Performance Benchmarks

**Test Environment**: Intel i5-8400 (6 cores), 8GB RAM, DDR4-2666

| Metric | Value | Notes |
|--------|-------|-------|
| Hashrate | 1-50 H/s | Depends on difficulty |
| Memory Usage | 200-400 MB | Stable |
| CPU Utilization | 80-90% | Memory bandwidth limited |
| Reconnection Backoff | 2s-60s | Exponential |

---

## Monitoring & Metrics

### Console Output

Real-time mining statistics:

```
2026-05-19 10:00:00 🚀 NogoMiner v1.0.0 starting
2026-05-19 10:00:01 ✅ Connected to pool: ws://localhost:1819/stratum
2026-05-19 10:00:02 ⛏️  Mining started with 4 threads
2026-05-19 10:00:12 📊 Hashrate: 1.25 KH/s | Accepted: 12 | Rejected: 0
2026-05-19 10:00:22 📊 Hashrate: 1.30 KH/s | Accepted: 25 | Rejected: 0
```

### Log Files

Logs are saved to `nogominer.log` with automatic rotation:

```
nogominer.log          # Current log
nogominer.log.1        # Rotated log
nogominer.log.2.gz     # Compressed rotated log
```

### Prometheus Metrics

Enable Prometheus export in config:

```json
{
  "monitor": {
    "prometheus_enabled": true,
    "prometheus_port": 9090
  }
}
```

Access metrics at `http://localhost:9090/metrics`:

```prometheus
# HELP nogominer_hashrate_current Current hashrate in H/s
# TYPE nogominer_hashrate_current gauge
nogominer_hashrate_current 1250.5

# HELP nogominer_shares_accepted Total accepted shares
# TYPE nogominer_shares_accepted counter
nogominer_shares_accepted 1250

# HELP nogominer_shares_rejected Total rejected shares
# TYPE nogominer_shares_rejected counter
nogominer_shares_rejected 3

# HELP nogominer_uptime_seconds Miner uptime in seconds
# TYPE nogominer_uptime_seconds counter
nogominer_uptime_seconds 3600
```

### Grafana Dashboard (Optional)

Import the provided Grafana dashboard JSON for visualization:

```json
{
  "dashboard": {
    "title": "NogoMiner Metrics",
    "panels": [
      {
        "title": "Hashrate",
        "targets": [
          {
            "expr": "nogominer_hashrate_current"
          }
        ]
      }
    ]
  }
}
```

---

## Performance Optimization

### CPU Optimization

```bash
# Linux: Set CPU affinity
taskset -c 0-3 ./nogominer

# Linux: Set process priority
nice -n -10 ./nogominer

# Windows: Set priority class
wmic process where "name='nogominer.exe'" CALL setpriority "high"
```

### Thread Configuration

Optimal thread count depends on CPU:

```json
{
  "miner": {
    "threads": 4  // Match physical cores (not hyperthreads)
  }
}
```

**Recommendations**:
- 2-core CPU: `threads: 2`
- 4-core CPU: `threads: 4`
- 6-core CPU: `threads: 6`
- 8+ core CPU: `threads: 6-8` (memory bandwidth limited)

### Memory Optimization

```json
{
  "miner": {
    "batch_size": 500  // Reduce if memory-constrained
  }
}
```

### Network Optimization

Use WebSocket for lower latency:

```json
{
  "rpc": {
    "ws_url": "ws://localhost:1819/stratum"
  }
}
```

### BIOS/UEFI Settings

For dedicated mining systems:

- Enable: `Above 4G Decoding`
- Enable: `Resizable BAR` (if supported)
- Disable: `C-States` (for constant performance)
- Set: `Memory Profile` (XMP/DOCP for rated speed)

---

## Troubleshooting

### Connection Issues

#### Problem: Unable to connect to pool

```
ERROR: failed to connect to pool: dial tcp: connection refused
```

**Solutions**:
1. Verify pool is running: `curl http://localhost:1818`
2. Check firewall rules: `sudo ufw status`
3. Confirm WebSocket URL is correct
4. Test network connectivity: `telnet localhost 1819`

#### Problem: Authentication failed

```
ERROR: authentication failed: invalid mining address
```

**Solutions**:
1. Verify address format (must start with "NOGO")
2. Check address length (78 characters)
3. Ensure address is valid NogoChain address

### Performance Issues

#### Problem: Low hashrate

**Possible Causes**:
- Insufficient CPU cores
- Memory bandwidth bottleneck
- High system load

**Solutions**:
1. Increase thread count (up to physical cores)
2. Close other memory-intensive applications
3. Upgrade to faster RAM
4. Use CPU with more cores

#### Problem: High rejection rate

```
WARNING: High rejection rate: 15%
```

**Solutions**:
1. Check network latency to pool
2. Reduce `batch_size` for faster submission
3. Use wired connection (not WiFi)
4. Verify system time is synchronized

### Memory Issues

#### Problem: Out of memory

```
panic: runtime error: out of memory
```

**Solutions**:
1. Reduce `threads` count
2. Reduce `batch_size`
3. Close other applications
4. Add more RAM

### Log Analysis

```bash
# View recent errors
tail -f nogominer.log | grep ERROR

# View connection issues
grep "connection" nogominer.log

# View share statistics
grep "Accepted\|Rejected" nogominer.log
```

---

## Security Best Practices

### Private Key Security

⚠️ **CRITICAL**: Never share your mining address private key!

```bash
# Store private key securely (offline recommended)
echo "YOUR_PRIVATE_KEY" > /secure/location/key.txt
chmod 600 /secure/location/key.txt

# NEVER commit keys to version control
echo "*.key" >> .gitignore
echo "private_key.txt" >> .gitignore
```

### Network Security

```bash
# Firewall configuration (Linux)
sudo ufw allow from 127.0.0.1 to any port 1818
sudo ufw allow from 127.0.0.1 to any port 1819

# Restrict access to trusted IPs
sudo ufw allow from 192.168.1.0/24 to any port 1818
```

### Production Deployment

1. **Use HTTPS/WSS** for encrypted connections
2. **Restrict RPC access** with firewall rules
3. **Monitor regularly** for suspicious activity
4. **Backup configuration** and keys securely
5. **Update regularly** for security patches

---

## Development

### Project Structure

```
Nogo-miner/
├── cmd/nogominer/       # Main program entry point
├── internal/            # Internal packages
│   ├── config/         # Configuration management
│   ├── logger/         # Logging system
│   ├── miner/          # Mining core logic
│   ├── monitor/        # Monitoring system
│   ├── pool/           # Pool manager
│   ├── rpc/            # RPC client
│   └── stratum/        # Stratum protocol client
├── pkg/nogopow/        # NogoPow algorithm implementation
├── configs/            # Configuration examples
├── go.mod              # Go module definition
└── go.sum              # Dependency checksums
```

### Building from Source

```bash
# Development build
go build -o nogominer ./cmd/nogominer

# Production build (optimized)
go build -ldflags="-s -w" -o nogominer ./cmd/nogominer

# Cross-compilation
GOOS=linux GOARCH=amd64 go build -o nogominer-linux ./cmd/nogominer
GOOS=windows GOARCH=amd64 go build -o nogominer.exe ./cmd/nogominer
GOOS=darwin GOARCH=amd64 go build -o nogominer-macos ./cmd/nogominer
```

### Testing

```bash
# Run all tests
go test ./...

# Run with race detection
go test -race ./...

# Run specific package tests
go test ./internal/miner/...

# Generate coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### Code Quality

```bash
# Format code
go fmt ./...

# Static analysis
go vet ./...

# Linting (requires golangci-lint)
golangci-lint run

# Security scanning (requires govulncheck)
govulncheck ./...
```

### Debugging

```bash
# Build with debug symbols
go build -gcflags="all=-N -l" -o nogominer-debug ./cmd/nogominer

# Run with Delve debugger
dlv debug ./cmd/nogominer

# Set breakpoints and inspect variables
(dlv) break main.go:42
(dlv) continue
```

---

## API Reference

### Internal APIs (for developers)

#### Stratum Client

```go
// Connect establishes WebSocket connection and starts readLoop
func (c *Client) Connect(ctx context.Context) error

// Close permanently stops readLoop and waits for cleanup
func (c *Client) Close() error

// IsConnected returns current connection status
func (c *Client) IsConnected() bool

// IsReconnecting returns true if dialAndLogin is in progress
func (c *Client) IsReconnecting() bool

// SubmitShare submits a share to the pool
func (c *Client) SubmitShare(ctx context.Context, jobID, nonce uint64) error

// SendHashReport sends incremental hash count to pool
func (c *Client) SendHashReport(ctx context.Context, hashes uint64) error

// GetJobChannel returns the job channel for receiving mining jobs
func (c *Client) GetJobChannel() <-chan *MiningJob

// GetResultChannel returns the result channel for submission results
func (c *Client) GetResultChannel() <-chan *SubmitResult

// GetCurrentJob returns the current mining job
func (c *Client) GetCurrentJob() *MiningJob
```

#### NogoPow Engine

```go
// Mine performs the core mining loop
func (e *Engine) Mine(headerHash []byte, seed []byte, difficulty uint64,
    nonceStart uint64, nonceEnd uint64, resultCh chan *MiningResult)

// VerifyPoW verifies a computed PoW solution
func (e *Engine) VerifyPoW(headerHash []byte, seed []byte,
    nonce uint64, difficulty uint64) bool
```

#### Miner

```go
// Start begins mining operations
func (m *Miner) Start(ctx context.Context) error

// Stop gracefully stops mining
func (m *Miner) Stop() error

// GetStats returns current mining statistics
func (m *Miner) GetStats() *MinerStats

// GetHashRate returns real-time hashrate
func (m *Miner) GetHashRate() uint64
```

#### Pool Manager

```go
// Start launches the pool manager and health check loop
func (m *Manager) Start(ctx context.Context) error

// Stop shuts down the pool manager
func (m *Manager) Stop()

// GetClient returns the Stratum client for the current pool
func (m *Manager) GetClient() *stratum.Client

// RecordShare records a share submission result
func (m *Manager) RecordShare(accepted bool)

// UpdateHashRate updates the current pool hash rate
func (m *Manager) UpdateHashRate(hashRate uint64)
```

---

## Contributing

We welcome contributions! Please follow these guidelines:

### How to Contribute

1. **Fork** the repository
2. **Create** a feature branch (`git checkout -b feature/amazing-feature`)
3. **Commit** your changes (`git commit -m 'Add amazing feature'`)
4. **Push** to the branch (`git push origin feature/amazing-feature`)
5. **Open** a Pull Request

### Code Standards

- Follow [Effective Go](https://golang.org/doc/effective_go.html)
- Use `gofmt` for formatting
- Write tests for new features
- Document public APIs
- Keep functions small and focused

### Reporting Issues

- Use GitHub Issues
- Provide detailed description
- Include steps to reproduce
- Add system information
- Attach logs if relevant

---

## License

This project is licensed under the **GNU Lesser General Public License v3.0** (LGPL-3.0).

See [LICENSE](LICENSE) for details.

### Summary

- ✅ Free to use for personal and commercial purposes
- ✅ Modifications must be released under same license
- ✅ Library linking allowed without copyleft

---

## Support

### Resources

- **Documentation**: https://docs.nogochain.io
- **GitHub Issues**: https://github.com/nogochain/nogo-miner/issues
- **Telegram**: https://t.me/nogochain
- **Discord**: https://discord.gg/nogochain
- **Website**: https://nogochain.io

### Contact

- **Email**: support@nogochain.io
- **Twitter**: @nogochain
- **Medium**: https://medium.com/@nogochain

---

## Acknowledgments

- NogoChain core team for algorithm specification
- Community contributors for testing and feedback
- Open-source mining software community

---

**Version**: v1.0.0  
**Last Updated**: 2026-06-09  
**Maintained By**: NogoChain Development Team

---

*Happy Mining! ⛏️*
