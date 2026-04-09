@echo off
echo ╔══════════════════════════════════════════════════════════╗
echo ║          NogoMiner 快速编译和启动                      ║
echo ╚══════════════════════════════════════════════════════════╝
echo.

cd /d "%~dp0"

echo [步骤 1/3] 检查环境...
where go >nul 2>nul
if errorlevel 1 (
    echo [错误] 未找到 Go 编译器
    echo 请安装 Go 1.21+ 从: https://golang.org/dl/
    pause
    exit /b 1
)
go version
echo.

echo [步骤 2/3] 编译矿工...
echo 执行: go build -o nogominer.exe -ldflags="-s -w" .\cmd\nogominer
if exist "nogominer.exe" (
    echo [提示] 发现旧的 nogominer.exe，删除中...
    del nogominer.exe
)

go build -o nogominer.exe -ldflags="-s -w" .\cmd\nogominer

if errorlevel 1 (
    echo [错误] 编译失败！
    pause
    exit /b 1
)
echo [成功] 编译完成
echo.

echo [步骤 3/3] 启动矿工...
echo 执行: .\nogominer.exe -config config.json
echo.
echo ╔══════════════════════════════════════════════════════════╗
echo ║                    矿工启动成功！                      ║
echo ║                                                          ║
echo ║  日志文件: nogominer.log                                ║
echo ║  停止矿工: Ctrl + C                                      ║
echo ║  查看状态: 运行 status.bat                              ║
echo ╚══════════════════════════════════════════════════════════╝
echo.

start "NogoMiner" nogominer.exe -config config.json

timeout /t 3 /nobreak >nul
echo.
echo 按任意键退出此窗口（矿工将继续在后台运行）...
pause >nul
