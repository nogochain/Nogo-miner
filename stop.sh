#!/bin/bash

# NogoMiner 停止脚本

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

echo "停止 NogoMiner..."

if [ -f "nogominer.pid" ]; then
    PID=$(cat nogominer.pid)
    if kill -0 $PID 2>/dev/null; then
        echo "发送停止信号到进程 $PID..."
        kill $PID
        
        # 等待进程结束
        for i in {1..10}; do
            if ! kill -0 $PID 2>/dev/null; then
                echo "✅ NogoMiner 已停止"
                rm -f nogominer.pid
                exit 0
            fi
            sleep 1
        done
        
        # 强制停止
        echo "强制停止进程..."
        kill -9 $PID
        rm -f nogominer.pid
        echo "✅ NogoMiner 已强制停止"
    else
        echo "⚠️  进程不存在，清理 PID 文件"
        rm -f nogominer.pid
    fi
else
    # 尝试通过进程名停止
    PID=$(pgrep -f "nogominer.*config.json" || true)
    if [ -n "$PID" ]; then
        echo "找到进程 $PID，停止中..."
        kill $PID
        echo "✅ NogoMiner 已停止"
    else
        echo "ℹ️  NogoMiner 未运行"
    fi
fi

exit 0
