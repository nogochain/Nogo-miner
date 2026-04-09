# Nogo-miner - NogoChain Mining Software

**Standalone mining software for NogoChain blockchain, fully compatible with NogoPow consensus algorithm**

## Table of Contents

- [Features](#features)
- [System Requirements](#system-requirements)
- [Installation](#installation)
- [Configuration](#configuration)
- [Usage](#usage)
- [Monitoring](#monitoring)
- [NogoPow Algorithm Implementation](#nogopow-algorithm-implementation)
- [Performance Optimization](#performance-optimization)
- [NogoPool Integration](#nogopool-integration)
- [Troubleshooting](#troubleshooting)
- [Development](#development)

## Features

- **Full NogoPow Algorithm Support**: Precise implementation of NogoChain's matrix-based proof-of-work algorithm
- **High-Performance Parallel Computing**: Multi-core CPU mining with optimized 256x256 matrix multiplication
- **Real-time Monitoring**: Live hash rate, accepted/rejected shares, and detailed statistics
- **Smart Reconnection**: Automatic reconnection after network failures with resume capability
- **Flexible Configuration**: Support for config files, command-line arguments, and environment variables
- **Production-Ready Stability**: Comprehensive error handling, resource management, and concurrency safety

## System Requirements

- **Operating System**: Windows 10+, Linux (Ubuntu 18.04+), macOS 10.15+
- **CPU**: Multi-core processor (recommended 4+ cores)
- **Memory**: Minimum 2GB, recommended 4GB+
- **Network**: Stable internet connection
- **Go Language**: Go 1.21.5+ (if compiling from source)

## Installation

### Method 1: Download Pre-compiled Binaries

Download pre-compiled binaries from [Releases](https://github.com/nogochain/nogo-miner/releases).

### Method 2: Source Code Compilation

```bash
# Clone repository
git clone https://github.com/nogochain/nogo-miner.git
cd nogo-miner

# Build
-go build -o nogominer -ldflags="-s -w" ./cmd/nogominer

# Build with race detection (for debugging)
go build -race -o nogominer ./cmd/nogominer
```

### Method 3: Docker (Coming Soon)

```bash
docker run -d --name nogominer \
  -v /path/to/config:/config \
  nogochain/nogo-miner:latest
```

## Configuration

### Configuration File

Copy the example configuration file:

```bash
cp configs/config.example.json config.json
```

Edit `config.json`:

```json
{
  "rpc": {
    "url": "http://localhost:8080",
    "ws_url": "ws://localhost:8080/ws",
    "timeout_seconds": 30
  },
  "pools": [
    {
      "name": "mainnet-pool",
      "url": "http://node.nogochain.io:8080",
      "address": "YOUR_MINING_ADDRESS",
      "priority": 1,
      "enabled": true
    }
  ],
  "miner": {
    "threads": 0,
    "batch_size": 1000
  },
  "logging": {
    "level": "info",
    "file": "nogominer.log"
  }
}
```

### Configuration Options

#### RPC Configuration
- `url`: NogoChain node HTTP RPC address
- `ws_url`: WebSocket RPC address (for real-time subscriptions)
- `timeout_seconds`: RPC request timeout
- `max_retries`: Maximum retry attempts
- `retry_delay_seconds`: Retry delay between attempts

#### Pool Configuration
- `name`: Pool name identifier
- `url`: Pool RPC address (format: http://host:port or stratum+tcp://host:port)
- `ws_url`: Pool WebSocket address
- `address`: Mining reward address (must be valid NogoChain address)
- `priority`: Priority (1=highest, lower numbers have higher priority)
- `enabled`: Whether to enable this pool connection

#### Miner Configuration
- `threads`: Number of mining threads (0=auto-detect CPU cores)
- `batch_size`: Batch size for share submission
- `share_difficulty`: Minimum share difficulty threshold

#### Logging Configuration
- `level`: Log level (debug, info, warn, error)
- `file`: Log file path
- `max_size_mb`: Maximum log file size
- `max_backups`: Maximum number of backup files
- `max_age_days`: Maximum log file age
- `json_format`: Whether to use JSON format

### NogoPool Configuration Example

To integrate with a locally running NogoPool:

```json
{
  "pools": [
    {
      "name": "local-nogopool",
      "url": "http://localhost:8080",
      "ws_url": "ws://localhost:8080/ws",
      "address": "nogo1q5z9p8h4fg5h9v6m7n8k9l0m1n2o3p4q5r6s7t8u9v0w1x2y3z4a5b6c7d8e9f0g",
      "priority": 1,
      "enabled": true
    }
  ]
}
```

For production environment with failover:
```json
{
  "pools": [
    {
      "name": "primary-pool",
      "url": "stratum+tcp://pool1.nogochain.io:8080",
      "address": "<your-nogochain-address>",
      "priority": 1,
      "enabled": true
    },
    {
      "name": "backup-pool", 
      "url": "stratum+tcp://pool2.nogochain.io:8080",
      "address": "<your-nogochain-address>",
      "priority": 2,
      "enabled": true
    }
  ]
}
```

## Usage

### Quick Start (Recommended)

**Linux/macOS:**
```bash
# First run (automatically creates config file)
./start.sh

# Check status
./status.sh

# Stop
./stop.sh
```

**Windows:**
```batch
REM First run (automatically creates config file)
start.bat

REM Check status
status.bat

REM Stop
stop.bat
```

### Basic Usage

```bash
# Use default config file
./nogominer

# Specify config file
./nogominer -config /path/to/config.json

# Override config (command-line args take precedence)
./nogominer -rpc-url http://node:8080 -threads 4

# Show help
./nogominer -h
```

### Command Line Arguments

```
-config string
      Config file path (default "config.json")
-rpc-url string
      RPC server URL
-ws-url string
      WebSocket URL
-threads int
      Number of mining threads (0 = auto-detect)
-address string
      Mining address
-log-level string
      Log level (default "info")
-version
      Display version information
```

### Environment Variables

```bash
# Set RPC address
export NOGOMINER_RPC_URL=http://localhost:8080

# Set mining address
export NOGOMINER_ADDRESS=<your-nogochain-address>

# Set thread count
export NOGOMINER_THREADS=4

# Start miner
./nogominer
```

## Monitoring

### Console Output

```
2026-04-06 21:27:00 🚀 NogoMiner v1.0.0 starting
2026-04-06 21:27:01 ✅ Connected to node: http://localhost:8080
2026-04-06 21:27:01 ⛏️  Mining started with 4 threads
2026-04-06 21:27:11 📊 Hashrate: 1.25 KH/s | Accepted: 12 | Rejected: 0
2026-04-06 21:27:21 📊 Hashrate: 1.30 KH/s | Accepted: 25 | Rejected: 0
```

### Log Files

Log files are saved to `nogominer.log` by default, with rotation support.

### Prometheus Monitoring (Optional)

Enable Prometheus metrics export:

```json
{
  "monitor": {
    "prometheus_enabled": true,
    "prometheus_port": 9090
  }
}
```

Access metrics at `http://localhost:9090/metrics`.

## NogoPow Algorithm Implementation

### Algorithm Overview

Nogo-miner fully supports NogoChain's NogoPow proof-of-work algorithm, maintaining exact synchronization with node implementations. The core algorithm is based on memory-intensive matrix operations, using 256x256 matrix multiplication as the core computation for proof-of-work.

### Core Algorithm Flow

```
Block Header → RLP Encoding → Keccak-256 Hash → Matrix Operations → Final Proof-of-Work
```

#### 1. Block Hash Calculation
```go
func (e *Engine) computeBlockHash(header *BlockHeader, nonce uint64) []byte {
    // Create block header structure
    blockHeader := &Header{
        ParentHash: BytesToHash(header.PrevHash),
        Number:     new(big.Int).SetUint64(header.Height),
        Time:       uint64(header.Timestamp),
        Nonce:      uint64ToBlockNonce(nonce),
        Difficulty: header.Difficulty,
    }
    
    // Calculate seal hash (RLP encoding + SHA3-256)
    blockHash := e.sealHash(blockHeader)
    
    // Compute PoW using cache (exactly matching node algorithm)
    powHash := e.computePoW(blockHash, seedFromParent(header.PrevHash))
    
    return powHash[:]
}
```

#### 2. Core Matrix Multiplication Algorithm
```go
func mulMatrix(headerHash []byte, cache []uint32) []uint8 {
    // 4-way parallel matrix computation (utilizing multi-core CPUs)
    runtime.GOMAXPROCS(4)
    var wg sync.WaitGroup
    wg.Add(4)
    
    for k := 0; k < 4; k++ {
        go func(i int) {
            defer wg.Done()
            // Each goroutine processes partial matrix computation
            // Generate random sequence based on headerHash
            var sequence [32]byte
            hasher := sha3.NewLegacyKeccak256()
            hasher.Write(headerHash[i*8 : (i+1)*8])
            copy(sequence[:], hasher.Sum(nil))
            
            // Build matrix chain and perform mixing operations
            for j := 0; j < 2; j++ {
                for k := 0; k < 32; k++ {
                    index := int(sequence[k])
                    // Matrix multiplication (blocked optimization)
                    mulMatrixBlocked(dst, localMatA.data, mb.data, matSize)
                }
            }
        }(k)
    }
    wg.Wait()
    
    return result
}
```

#### 3. Difficulty Target Calculation
```go
func difficultyToTarget(difficulty *big.Int) *big.Int {
    maxTarget := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))
    target := new(big.Int).Div(maxTarget, difficulty)
    return target
}
```

### Key Technical Features

#### 1. Exact Algorithm Matching
- **Matrix Dimensions**: 256x256 matrices, exactly matching node implementation
- **Fixed-point Arithmetic**: 30-bit fixed-point precision to avoid floating-point errors
- **Cache Mechanism**: Intelligent seed-based cache reuse for improved computational efficiency

#### 2. Parallel Computing Optimization
- **4-way Parallelism**: Uses 4 goroutines for parallel matrix operations
- **Memory Pool**: Matrix object reuse to minimize memory allocation overhead
- **Blocked Algorithm**: 32x32 blocked matrix multiplication for better cache performance

#### 3. Security and Stability
- **RLP Encoding**: Exact same serialization format as nodes
- **Hash Algorithm**: Keccak-256 algorithm identical to node implementation
- **Boundary Checks**: Comprehensive boundary checks and error handling

### Algorithm Consistency Verification

The mining software has passed the following key validations:
1. **Constant Consistency**: Matrix size (256x256), block size (32) match node implementation
2. **Algorithm Flow**: RLP encoding, hash calculation, matrix multiplication order matches node
3. **Difficulty Target**: Difficulty to target conversion algorithm matches node
4. **Cache Mechanism**: Seed cache generation and reuse logic matches node

### Performance Benchmarks

Under standard configuration (4-core CPU, 8GB RAM):
- **Average Hash Rate**: 1.2-1.5 KH/s
- **Matrix Computation Throughput**: ~1500 256x256 matrix multiplications/second
- **Memory Usage**: Stable at 200-400MB
- **CPU Utilization**: 80-90% (memory bandwidth limited)

## Performance Optimization

### CPU Optimization

```bash
# Set CPU affinity (Linux)
taskset -c 0-3 ./nogominer

# Set priority (Linux)
nice -n -10 ./nogominer
```

### Memory Optimization

Adjust `batch_size` to reduce memory allocation:

```json
{
  "miner": {
    "batch_size": 500
  }
}
```

### Network Optimization

Use WebSocket subscriptions to reduce latency:

```json
{
  "rpc": {
    "ws_url": "ws://localhost:8080/ws"
  }
}
```

## NogoPool Integration

### Integration Overview

Nogo-miner communicates with NogoPool mining pool using standard Stratum protocol, ensuring efficient and stable mining collaboration. The miner is responsible for computing proof-of-work, while the pool handles task distribution, result validation, and profit distribution.

### Connection Workflow

1. **Initial Connection**: Miner connects to NogoPool via Stratum protocol
2. **Authentication**: Miner authenticates using configured mining address
3. **Task Reception**: Pool distributes latest mining tasks (block headers, difficulty, etc.)
4. **Mining Computation**: Miner computes valid nonce using NogoPow algorithm
5. **Result Submission**: Submit valid solution to pool for verification
6. **Profit Calculation**: Pool calculates rewards based on submitted shares

### Configuration Guide

#### 1. Basic Configuration
```json
{
  "pools": [
    {
      "name": "Primary Pool",
      "url": "stratum+tcp://192.168.1.100:8080",
      "address": "Your NogoChain Address",
      "priority": 1,
      "enabled": true
    }
  ],
  "miner": {
    "name": "My Miner",
    "threads": 4
  }
}
```

#### 2. Advanced Configuration (High Availability)
```json
{
  "pools": [
    {
      "name": "Primary Pool",
      "url": "stratum+tcp://primary.nogochain.io:8080",
      "ws_url": "ws://primary.nogochain.io:8080/ws",
      "address": "<your-address>",
      "priority": 1,
      "enabled": true
    },
    {
      "name": "Backup Pool",
      "url": "stratum+tcp://backup.nogochain.io:8080", 
      "address": "<your-address>",
      "priority": 2,
      "enabled": true
    }
  ]
}
```

### Connection Verification

Successful connection will display:
```
[INFO] Connected to pool: stratum+tcp://localhost:8080
[INFO] Authentication successful - Miner address: nogo1q5z9p8h4fg...
[INFO] Received mining task - Height: 12345, Difficulty: 1000000
[INFO] Started 4 mining threads
[INFO] Current Hashrate: 1.25 KH/s
[INFO] Accepted shares: 15, Rejected shares: 0
```

### Key Monitoring Metrics

- **Hash Rate**: Reflects mining computational power
- **Accepted Shares**: Proof-of-work successfully validated by pool
- **Rejected Shares**: Submissions rejected due to staleness or invalidity
- **Latency**: Communication delay with the pool
- **Uptime**: Duration of stable operation

### Best Practices

1. **Address Validation**: Ensure configured address is a valid NogoChain address
2. **Network Stability**: Use wired connections to avoid WiFi fluctuations
3. **Failover Configuration**: Configure multiple pools for high availability
4. **Performance Monitoring**: Regularly check hash rate and rejection rate
5. **Security Configuration**: Use encrypted connections (HTTPS/WSS) in production

## Troubleshooting

### Unable to Connect to Node

```
ERROR: failed to connect to node: connection refused
```

**Solutions:**
1. Check if node is running: `curl http://localhost:8080`
2. Check firewall settings
3. Confirm RPC address is correct

### Invalid Mining Address

```
ERROR: invalid mining address format
```

**Solutions:**
1. Confirm address starts with "nogo"
2. Address length should be 78 characters
3. Check address checksum

### Out of Memory

```
panic: runtime error: out of memory
```

**Solutions:**
1. Reduce `threads` count
2. Reduce `batch_size`
3. Close other memory-intensive applications

### Low Hash Rate

**Possible Causes:**
1. Insufficient CPU cores
2. Memory bandwidth bottleneck
3. High system load

**Solutions:**
1. Increase `threads` (but not beyond CPU core count)
2. Optimize system performance
3. Use faster CPU

## Development

### Project Structure

```
Nogo-miner/
├── cmd/nogominer/     # Main program entry
├── internal/          # Internal modules
│   ├── config/       # Configuration management
│   ├── logger/       # Logging system
│   ├── rpc/          # RPC client
│   ├── miner/        # Mining core
│   ├── pool/         # Pool management
│   └── monitor/      # Monitoring system
├── pkg/nogopow/      # NogoPow algorithm
├── configs/          # Configuration files
└── go.mod
```

### Building

```bash
# Development build
go build ./cmd/nogominer

# Production build
go build -ldflags="-s -w" ./cmd/nogominer

# Cross-compilation
GOOS=linux GOARCH=amd64 go build -o nogominer-linux ./cmd/nogominer
GOOS=windows GOARCH=amd64 go build -o nogominer.exe ./cmd/nogominer
```

### Testing

```bash
# Unit tests
go test ./...

# Race detection
go test -race ./...

# Coverage
go test -cover ./...
```

### Code Standards

```bash
# Formatting
go fmt ./...

# Static analysis
go vet ./...

# Linting (requires golangci-lint)
golangci-lint run
```

## Security Considerations

1. **Private Key Protection**: Never share your mining address private key
2. **Use HTTPS**: Encrypted connections in production environments
3. **Firewall**: Restrict RPC port access
4. **Monitoring**: Regularly check mining rewards and system status

## License

GNU Lesser General Public License v3.0

## Support

- Documentation: https://docs.nogochain.io
- Telegram: https://t.me/nogochain
- GitHub Issues: https://github.com/nogochain/nogo-miner/issues

## Contributing

Welcome to submit issues and pull requests!

---

**Version**: v1.0.0  
**Updated**: 2026-04-06