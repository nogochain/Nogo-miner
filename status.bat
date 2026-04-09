@echo off
REM NogoMiner 状态脚本

cd /d "%~dp0"

echo ╔══════════════════════════════════════════════════════════╗
echo ║          NogoMiner 状态                                  ║
echo ╚══════════════════════════════════════════════════════════╝
echo.

REM 检查进程
tasklist /FI "IMAGENAME eq nogominer.exe" 2>nul | findstr /I "nogominer.exe" >nul 2>nul
if not errorlevel 1 (
    echo 状态：[运行中]
    
    REM 获取进程信息
    for /f "tokens=2" %%i in ('tasklist /FI "IMAGENAME eq nogominer.exe" /NH /FO CSV') do set PID=%%i
    echo PID: %PID%
    
    REM 内存使用
    for /f "tokens=5" %%i in ('tasklist /FI "IMAGENAME eq nogominer.exe" /NH /FO CSV') do set MEM=%%i
    echo 内存：!MEM!
    
    echo.
    
    REM 日志最后 10 行
    if exist "nogominer.log" (
        echo 最新日志:
        powershell -Command "Get-Content nogominer.log -Tail 10"
    )
) else (
    echo 状态：[未运行]
)

echo.

REM 磁盘空间
echo 磁盘空间:
dir . | findstr "可用字节"

REM 日志文件大小
if exist "nogominer.log" (
    for %%A in (nogominer.log) do echo 日志文件：%%~zA 字节
)

echo.
pause
