@echo off
REM NogoMiner 快速启动脚本 for Windows

setlocal enabledelayedexpansion

cd /d "%~dp0"

echo ╔══════════════════════════════════════════════════════════╗
echo ║          NogoMiner 快速启动脚本                      ║
echo ╚══════════════════════════════════════════════════════════╝
echo.

REM 检查配置文件
if not exist "config.json" (
    if exist "configs\config.example.json" (
        echo [警告] 配置文件不存在，从示例创建...
        copy configs\config.example.json config.json
        echo [警告] 请编辑 config.json 配置您的矿池和地址
        echo.
        echo 配置完成后，再次运行此脚本。
        pause
        exit /b 1
    ) else (
        echo [错误] 找不到配置文件
        pause
        exit /b 1
    )
)

REM 检查二进制文件
if not exist "nogominer.exe" (
    echo [提示] 二进制文件不存在，开始编译...
    
    REM 检查 Go
    where go >nul 2>nul
    if errorlevel 1 (
        echo [错误] 未找到 Go 编译器
        echo 请安装 Go 1.21+: https://golang.org/dl/
        pause
        exit /b 1
    )
    
    REM 编译
    echo 编译中...
    go build -ldflags="-s -w" -o nogominer.exe .\cmd\nogominer
    
    if errorlevel 1 (
        echo [错误] 编译失败
        pause
        exit /b 1
    ) else (
        echo [成功] 编译成功
    )
)

REM 显示配置信息
echo [信息] 配置信息:
echo   配置文件：config.json
echo   二进制：nogominer.exe
echo.

REM 检查是否已在运行
tasklist /FI "IMAGENAME eq nogominer.exe" 2>nul | findstr /I "nogominer.exe" >nul 2>nul
if not errorlevel 1 (
    echo [警告] NogoMiner 已在运行
    set /p restart="是否重启？(Y/N): "
    if /i not "!restart!"=="Y" (
        exit /b 0
    )
    
    REM 停止旧进程
    echo 停止旧进程...
    taskkill /F /IM nogominer.exe
    timeout /t 2 /nobreak >nul
)

REM 启动
echo 启动 NogoMiner...
start "NogoMiner" nogominer.exe -config config.json

echo [成功] 启动成功
echo.
echo [提示]
echo   - 查看日志：打开 nogominer.log
echo   - 停止服务：运行 stop.bat
echo   - 查看状态：运行 status.bat
echo.
echo 按任意键打开日志文件...
pause >nul

if exist "nogominer.log" (
    notepad nogominer.log
)

endlocal
