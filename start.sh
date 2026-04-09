#!/bin/bash

# NogoMiner 快速启动脚本 for Linux/macOS

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}╔══════════════════════════════════════════════════════════╗${NC}"
echo -e "${GREEN}║${NC}          NogoMiner 快速启动脚本                      ${GREEN}║${NC}"
echo -e "${GREEN}╚══════════════════════════════════════════════════════════╝${NC}"
echo ""

# 检查配置文件
if [ ! -f "config.json" ]; then
    if [ -f "configs/config.example.json" ]; then
        echo -e "${YELLOW}⚠️  配置文件不存在，从示例创建...${NC}"
        cp configs/config.example.json config.json
        echo -e "${YELLOW}⚠️  请编辑 config.json 配置您的矿池和地址${NC}"
        echo ""
        echo "配置完成后，再次运行此脚本。"
        exit 1
    else
        echo -e "${RED}❌ 错误：找不到配置文件${NC}"
        exit 1
    fi
fi

# 检查二进制文件
if [ ! -f "nogominer" ]; then
    echo -e "${YELLOW}⚠️  二进制文件不存在，开始编译...${NC}"
    
    # 检查 Go
    if ! command -v go &> /dev/null; then
        echo -e "${RED}❌ 错误：未找到 Go 编译器${NC}"
        echo "请安装 Go 1.21+: https://golang.org/dl/"
        exit 1
    fi
    
    # 编译
    echo "编译中..."
    go build -ldflags="-s -w" -o nogominer ./cmd/nogominer
    
    if [ $? -eq 0 ]; then
        echo -e "${GREEN}✅ 编译成功${NC}"
    else
        echo -e "${RED}❌ 编译失败${NC}"
        exit 1
    fi
fi

# 显示配置信息
echo -e "${GREEN}📋 配置信息:${NC}"
echo "  配置文件：config.json"
echo "  二进制：nogominer"
echo ""

# 检查是否是首次运行
if [ ! -f "nogominer.pid" ]; then
    echo -e "${GREEN}🚀 首次启动 NogoMiner...${NC}"
else
    # 检查是否已在运行
    if [ -f "nogominer.pid" ] && kill -0 $(cat nogominer.pid) 2>/dev/null; then
        echo -e "${YELLOW}⚠️  NogoMiner 已在运行 (PID: $(cat nogominer.pid))${NC}"
        read -p "是否重启？(y/N): " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            exit 0
        fi
        
        # 停止旧进程
        echo "停止旧进程..."
        kill $(cat nogominer.pid)
        sleep 2
    fi
fi

# 启动
echo -e "${GREEN}🚀 启动 NogoMiner...${NC}"
./nogominer -config config.json &
PID=$!
echo $PID > nogominer.pid

echo -e "${GREEN}✅ 启动成功 (PID: $PID)${NC}"
echo ""
echo -e "${YELLOW}提示:${NC}"
echo "  - 查看日志：tail -f nogominer.log"
echo "  - 停止服务：./stop.sh"
echo "  - 查看状态：./status.sh"
echo ""
echo -e "${GREEN}按 Ctrl+C 查看实时日志（或后台运行）${NC}"

# 等待并显示日志
wait $PID
