@echo off
REM NogoMiner 停止脚本

cd /d "%~dp0"

echo 停止 NogoMiner...

tasklist /FI "IMAGENAME eq nogominer.exe" 2>nul | findstr /I "nogominer.exe" >nul 2>nul
if not errorlevel 1 (
    echo 找到进程，停止中...
    taskkill /F /IM nogominer.exe
    
    if errorlevel 1 (
        echo [错误] 停止失败
    ) else (
        echo [成功] NogoMiner 已停止
    )
) else (
    echo [信息] NogoMiner 未运行
)

pause
