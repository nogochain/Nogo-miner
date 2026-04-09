@echo off
REM NogoMiner 矿池模式启动脚本

cd /d "%~dp0"

echo ╔══════════════════════════════════════════════════════════╗
echo ║          NogoMiner 矿池模式启动                        ║
echo ╚══════════════════════════════════════════════════════════╝
echo.

REM 检查配置文件
if not exist "config.json" (
    echo [错误] 找不到 config.json 配置文件
    echo.
    pause
    exit /b 1
)

REM 检查二进制文件
if not exist "nogominer.exe" (
    echo [错误] 找不到 nogominer.exe
    echo.
    pause
    exit /b 1
)

REM 显示配置信息
echo [信息] 配置信息:
echo   配置文件：config.json
echo   二进制：nogominer.exe
echo   矿池地址：ws://127.0.0.1:3333/stratum
echo.

REM 启动
echo 启动 NogoMiner...
echo.
echo 日志将输出到: nogominer.log
echo.
start "NogoMiner Pool Mode" nogominer.exe -config config.json

timeout /t 2 /nobreak >nul

echo [成功] 启动成功
echo.
echo [提示]
echo   - 查看日志: type nogominer.log
echo   - 实时监控: start.bat
echo   - 停止服务: stop.bat
echo.
echo 按任意键退出...
pause >nul

endlocal
