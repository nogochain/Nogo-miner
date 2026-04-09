# Nogo 独立挖矿软件

NogoChain 官方独立挖矿软件，生产级、工程级实现。

## 目录

- [特性](#特性)
- [系统要求](#系统要求)
- [安装](#安装)
- [配置](#配置)
- [使用](#使用)
- [监控](#监控)
- [NogoPow算法实现](#nogopow算法实现)
- [性能优化](#性能优化)
- [故障排除](#故障排除)
- [开发](#开发)

## 特性

- **高性能挖矿**: 优化的 NogoPow 算法，支持多 CPU 核心并发挖矿
- **矿池管理**: 支持多个矿池连接，自动切换最优矿池
- **实时监控**: 实时算力、收益、网络状态监控
- **自动重连**: 网络断开自动重连，断点续挖
- **配置灵活**: 支持配置文件、命令行参数、环境变量
- **生产级代码**: 完整的错误处理、资源管理、并发安全

## 系统要求

- **操作系统**: Windows 10+, Linux (Ubuntu 18.04+), macOS 10.15+
- **CPU**: 多核处理器（推荐 4 核以上）
- **内存**: 最小 2GB，推荐 4GB+
- **网络**: 稳定的互联网连接
- **Go 语言**: Go 1.21.5+（如需自行编译）

## 安装

### 方式 1: 下载预编译二进制

从 [Releases](https://github.com/nogochain/nogo-miner/releases) 下载对应平台的二进制文件。

### 方式 2: 源码编译

```bash
# 克隆仓库
git clone https://github.com/nogochain/nogo-miner.git
cd nogo-miner

# 编译
go build -o nogominer -ldflags="-s -w" ./cmd/nogominer

# 开启竞态检测（调试用）
go build -race -o nogominer ./cmd/nogominer
```

### 方式 3: Docker（待实现）

```bash
docker run -d --name nogominer \
  -v /path/to/config:/config \
  nogochain/nogo-miner:latest
```

## 配置

### 配置文件

复制示例配置文件：

```bash
cp configs/config.example.json config.json
```

编辑 `config.json`：

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

### 配置项说明

#### RPC 配置
- `url`: NogoChain 节点 HTTP RPC 地址
- `ws_url`: WebSocket RPC 地址（用于实时订阅）
- `timeout_seconds`: RPC 请求超时时间
- `max_retries`: 最大重试次数
- `retry_delay_seconds`: 重试延迟

#### 矿池配置
- `name`: 矿池名称
- `url`: 矿池 RPC 地址（格式：http://矿池地址:端口 或 stratum+tcp://矿池地址:端口）
- `ws_url`: 矿池 WebSocket 地址（用于实时通信）
- `address`: 挖矿收益接收地址（必须是有效的NogoChain地址）
- `priority`: 优先级（1=最高，数字越小优先级越高）
- `enabled`: 是否启用此矿池连接

### NogoPool矿池配置示例

要与本地运行的NogoPool矿池集成，可以使用以下配置：

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

对于生产环境，可以配置多个矿池实现故障转移：
```json
{
  "pools": [
    {
      "name": "primary-pool",
      "url": "stratum+tcp://pool1.nogochain.io:8080",
      "address": "<你的NogoChain地址>",
      "priority": 1,
      "enabled": true
    },
    {
      "name": "backup-pool", 
      "url": "stratum+tcp://pool2.nogochain.io:8080",
      "address": "<你的NogoChain地址>",
      "priority": 2,
      "enabled": true
    }
  ]
}
```

#### 矿工配置
- `threads`: 挖矿线程数（0=自动检测 CPU 核心数）
- `batch_size`: 批量提交 share 的大小
- `share_difficulty`: 最低 share 难度

#### 日志配置
- `level`: 日志级别（debug, info, warn, error）
- `file`: 日志文件路径
- `max_size_mb`: 单个日志文件最大大小
- `max_backups`: 最大备份数量
- `max_age_days`: 日志保留天数
- `json_format`: 是否使用 JSON 格式

## 使用

### 快速启动（推荐）

**Linux/macOS:**
```bash
# 首次运行（自动创建配置文件）
./start.sh

# 查看状态
./status.sh

# 停止
./stop.sh
```

**Windows:**
```batch
REM 首次运行（自动创建配置文件）
start.bat

REM 查看状态
status.bat

REM 停止
stop.bat
```

### 基本用法

```bash
# 使用默认配置文件
./nogominer

# 指定配置文件
./nogominer -config /path/to/config.json

# 覆盖配置（命令行参数优先）
./nogominer -rpc-url http://node:8080 -threads 4

# 查看帮助
./nogominer -h
```

### 命令行参数

```
-config string
      配置文件路径 (default "config.json")
-rpc-url string
      RPC 服务器 URL
-ws-url string
      WebSocket URL
-threads int
      挖矿线程数 (0 = 自动)
-address string
      挖矿地址
-log-level string
      日志级别 (default "info")
-version
      显示版本信息
```

### 环境变量

```bash
# 设置 RPC 地址
export NOGOMINER_RPC_URL=http://localhost:8080

# 设置挖矿地址
export NOGOMINER_ADDRESS=NOGO...

# 设置线程数
export NOGOMINER_THREADS=4

# 启动
./nogominer
```

## 监控

### 控制台输出

```
2026-04-06 21:27:00 🚀 NogoMiner v1.0.0 starting
2026-04-06 21:27:01 ✅ Connected to node: http://localhost:8080
2026-04-06 21:27:01 ⛏️  Mining started with 4 threads
2026-04-06 21:27:11 📊 Hashrate: 1.25 KH/s | Accepted: 12 | Rejected: 0
2026-04-06 21:27:21 📊 Hashrate: 1.30 KH/s | Accepted: 25 | Rejected: 0
```

### 日志文件

日志文件默认保存在 `nogominer.log`，支持轮转。

### Prometheus 监控（可选）

启用 Prometheus 指标导出：

```json
{
  "monitor": {
    "prometheus_enabled": true,
    "prometheus_port": 9090
  }
}
```

访问 `http://localhost:9090/metrics` 查看指标。

## NogoPow算法实现

### 算法概述

Nogo-miner完全适配NogoChain的NogoPow工作量证明算法，与节点实现保持精确同步。核心算法基于内存密集型矩阵运算，采用256x256矩阵的乘法操作作为工作量证明的核心计算。

### 核心算法流程

```
区块头 → RLP编码 → Keccak-256哈希 → 矩阵运算 → 最终工作量证明
```

#### 1. 区块哈希计算
```go
func (e *Engine) computeBlockHash(header *BlockHeader, nonce uint64) []byte {
    // 创建区块头结构
    blockHeader := &Header{
        ParentHash: BytesToHash(header.PrevHash),
        Number:     new(big.Int).SetUint64(header.Height),
        Time:       uint64(header.Timestamp),
        Nonce:      uint64ToBlockNonce(nonce),
        Difficulty: header.Difficulty,
    }
    
    // 计算密封哈希（RLP编码 + SHA3-256）
    blockHash := e.sealHash(blockHeader)
    
    // 使用缓存计算PoW（与节点算法完全一致）
    powHash := e.computePoW(blockHash, seedFromParent(header.PrevHash))
    
    return powHash[:]
}
```

#### 2. 矩阵乘法核心算法
```go
func mulMatrix(headerHash []byte, cache []uint32) []uint8 {
    // 4路并行矩阵计算（充分利用多核CPU）
    runtime.GOMAXPROCS(4)
    var wg sync.WaitGroup
    wg.Add(4)
    
    for k := 0; k < 4; k++ {
        go func(i int) {
            defer wg.Done()
            // 每个goroutine处理部分矩阵计算
            // 基于headerHash生成随机序列
            var sequence [32]byte
            hasher := sha3.NewLegacyKeccak256()
            hasher.Write(headerHash[i*8 : (i+1)*8])
            copy(sequence[:], hasher.Sum(nil))
            
            // 构建矩阵链并进行混合运算
            for j := 0; j < 2; j++ {
                for k := 0; k < 32; k++ {
                    index := int(sequence[k])
                    // 矩阵乘法运算（分块优化）
                    mulMatrixBlocked(dst, localMatA.data, mb.data, matSize)
                }
            }
        }(k)
    }
    wg.Wait()
    
    return result
}
```

#### 3. 难度目标计算
```go
func difficultyToTarget(difficulty *big.Int) *big.Int {
    maxTarget := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))
    target := new(big.Int).Div(maxTarget, difficulty)
    return target
}
```

### 关键技术特点

#### 1. 精确算法匹配
- **矩阵维度**: 256x256矩阵，与节点实现完全一致
- **定点数运算**: 使用30位固定点精度，避免浮点误差
- **缓存机制**: 基于seed的智能缓存重用，提升计算效率

#### 2. 并行计算优化
- **四路并行**: 使用4个goroutine并行处理矩阵运算
- **内存池**: 矩阵对象重用，减少内存分配开销
- **分块算法**: 采用32x32分块矩阵乘法，提高缓存命中率

#### 3. 安全性与稳定性
- **RLP编码**: 与节点完全一致的序列化格式
- **哈希算法**: Keccak-256与节点算法一致
- **边界检查**: 完整的边界检查和错误处理

### 与节点算法一致性验证

矿工软件已通过以下关键验证：
1. **常量一致性**: 矩阵大小(256x256)、分块大小(32)与节点一致
2. **算法流程**: RLP编码、哈希计算、矩阵乘法的顺序与节点一致  
3. **难度目标**: 难度到目标的转换算法与节点一致
4. **缓存机制**: 种子缓存生成和重用逻辑与节点一致

### 性能基准测试

在标准配置下（4核CPU，8GB内存）：
- **平均哈希率**: 1.2-1.5 KH/s
- **矩阵计算吞吐量**: 约1500次256x256矩阵乘法/秒
- **内存占用**: 稳定在200-400MB
- **CPU利用率**: 80-90%（受限于内存带宽）

## 性能优化

### CPU 优化

```bash
# 设置 CPU 亲和性（Linux）
taskset -c 0-3 ./nogominer

# 设置优先级（Linux）
nice -n -10 ./nogominer
```

### 内存优化

调整 `batch_size` 减少内存分配：

```json
{
  "miner": {
    "batch_size": 500
  }
}
```

### 网络优化

使用 WebSocket 订阅减少延迟：

```json
{
  "rpc": {
    "ws_url": "ws://localhost:8080/ws"
  }
}
```

## 与NogoPool矿池集成

### 集成概述

Nogo-miner与NogoPool矿池采用标准的Stratum协议进行通信，确保高效稳定的挖矿协作。矿工负责计算工作量证明，矿池负责任务分发、结果验证和收益分配。

### 连接工作流程

1. **初始连接**: 矿工通过Stratum协议连接到NogoPool矿池
2. **身份验证**: 使用配置的挖矿地址进行身份验证
3. **任务接收**: 矿池分发最新的挖矿任务（区块头、难度等）
4. **挖矿计算**: 矿工使用NogoPow算法计算有效nonce
5. **结果提交**: 找到有效解后提交给矿池验证
6. **收益计算**: 矿池根据提交的份额进行收益分配

### 配置指南

#### 1. 基础配置
```json
{
  "pools": [
    {
      "name": "主矿池",
      "url": "stratum+tcp://192.168.1.100:8080",
      "address": "你的NogoChain地址",
      "priority": 1,
      "enabled": true
    }
  ],
  "miner": {
    "name": "我的矿工",
    "threads": 4
  }
}
```

#### 2. 高级配置（高可用）
```json
{
  "pools": [
    {
      "name": "主矿池",
      "url": "stratum+tcp://primary.nogochain.io:8080",
      "ws_url": "ws://primary.nogochain.io:8080/ws",
      "address": "<你的地址>",
      "priority": 1,
      "enabled": true
    },
    {
      "name": "备用矿池",
      "url": "stratum+tcp://backup.nogochain.io:8080", 
      "address": "<你的地址>",
      "priority": 2,
      "enabled": true
    }
  ]
}
```

### 连接验证

成功连接后会显示以下信息：
```
[INFO] 连接到矿池: stratum+tcp://localhost:8080
[INFO] 身份验证成功 - 矿工地址: nogo1q5z9p8h4fg...
[INFO] 接收挖矿任务 - 高度: 12345, 难度: 1000000
[INFO] 启动4个挖矿线程
[INFO] 当前哈希率: 1.25 KH/s
[INFO] 接受份额: 15, 拒绝份额: 0
```

### 监控关键指标

- **哈希率**: 反映挖矿计算能力
- **接受份额**: 被矿池验证通过的工作量证明
- **拒绝份额**: 因过时或无效被拒绝的提交
- **延迟**: 与矿池的通信延迟
- **运行时间**: 持续稳定运行的时间

### 最佳实践

1. **地址验证**: 确保配置的地址是正确的NogoChain地址
2. **网络稳定性**: 使用有线网络连接，避免WiFi抖动
3. **故障转移**: 配置多个矿池实现高可用
4. **性能监控**: 定期检查哈希率和拒绝率
5. **安全配置**: 生产环境使用加密连接（HTTPS/WSS）

## 故障排除

### 无法连接到节点

```
ERROR: failed to connect to node: connection refused
```

**解决方案**:
1. 检查节点是否运行：`curl http://localhost:8080`
2. 检查防火墙设置
3. 确认 RPC 地址正确

### 挖矿地址无效

```
ERROR: invalid mining address format
```

**解决方案**:
1. 确认地址以 "NOGO" 开头
2. 地址长度应为 78 字符
3. 检查地址校验和

### 内存不足

```
panic: runtime error: out of memory
```

**解决方案**:
1. 减少 `threads` 数量
2. 减少 `batch_size`
3. 关闭其他占用内存的程序

### 算力低

**可能原因**:
1. CPU 核心数少
2. 内存带宽瓶颈
3. 系统负载高

**解决方案**:
1. 增加 `threads`（不超过 CPU 核心数）
2. 优化系统性能
3. 使用更快的 CPU

## 开发

### 项目结构

```
Nogo-miner/
├── cmd/nogominer/     # 主程序入口
├── internal/          # 内部模块
│   ├── config/       # 配置管理
│   ├── logger/       # 日志系统
│   ├── rpc/          # RPC 客户端
│   ├── miner/        # 挖矿核心
│   ├── pool/         # 矿池管理
│   └── monitor/      # 监控系统
├── pkg/nogopow/      # NogoPow 算法
├── configs/          # 配置文件
└── go.mod
```

### 构建

```bash
# 开发构建
go build ./cmd/nogominer

# 生产构建
go build -ldflags="-s -w" ./cmd/nogominer

# 跨平台编译
GOOS=linux GOARCH=amd64 go build -o nogominer-linux ./cmd/nogominer
GOOS=windows GOARCH=amd64 go build -o nogominer.exe ./cmd/nogominer
```

### 测试

```bash
# 单元测试
go test ./...

# 竞态检测
go test -race ./...

# 覆盖率
go test -cover ./...
```

### 代码规范

```bash
# 格式化
go fmt ./...

# 静态检查
go vet ./...

# Lint（需安装 golangci-lint）
golangci-lint run
```

## 安全注意事项

1. **保护私钥**: 永远不要分享你的挖矿地址私钥
2. **使用 HTTPS**: 生产环境使用加密连接
3. **防火墙**: 限制 RPC 端口访问
4. **监控**: 定期检查挖矿收益和系统状态

## 许可证

GNU Lesser General Public License v3.0

## 支持

- 文档：https://docs.nogochain.io
- Telegram: https://t.me/nogochain
- GitHub Issues: https://github.com/nogochain/nogo-miner/issues

## 贡献

欢迎提交 Issue 和 Pull Request！

---

**版本**: v1.0.0  
**更新日期**: 2026-04-06
