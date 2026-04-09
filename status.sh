#!/bin/bash

# NogoMiner 状态脚本

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

echo "╔══════════════════════════════════════════════════════════╗"
echo "║          NogoMiner 状态                                  ║"
echo "╚══════════════════════════════════════════════════════════╝"
echo ""

# 检查进程
if [ -f "nogominer.pid" ] && kill -0 $(cat nogominer.pid) 2>/dev/null; then
    PID=$(cat nogominer.pid)
    echo "状态: ✅ 运行中"
    echo "PID: $PID"
    echo "启动时间: $(ps -p $PID -o lstart=)"
    echo ""
    
    # CPU 和内存
    ps -p $PID -o pid,pcpu,pmem,rss,vsz,etime,cmd
    echo ""
    
    # 日志最后 10 行
    if [ -f "nogominer.log" ]; then
        echo "最新日志:"
        tail -n 10 nogominer.log
    fi
else
    echo "状态: ❌ 未运行"
    
    # 检查是否有 PID 文件但进程不存在
    if [ -f "nogominer.pid" ]; then
        echo "警告: PID 文件存在但进程不存在"
        rm -f nogominer.pid
    fi
fi

echo ""

# 磁盘空间
echo "磁盘使用:"
df -h . | tail -n 1

# 日志文件大小
if [ -f "nogominer.log" ]; then
    echo "日志文件: $(du -h nogominer.log | cut -f1)"
fi

exit 0
