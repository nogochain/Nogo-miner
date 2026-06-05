@echo off
REM =============================================================================
REM NogoMiner Windows Startup Script
REM Double-click to start NogoMiner with config.json
REM Production-grade implementation with error handling
REM =============================================================================

setlocal EnableDelayedExpansion

REM Script configuration
set "SCRIPT_NAME=%~nx0"
set "SCRIPT_DIR=%~dp0"
set "EXE_NAME=nogominer.exe"
set "CONFIG_NAME=config.json"
set "LOG_NAME=nogominer.log"

REM Change to script directory (ensure relative paths work)
cd /d "%SCRIPT_DIR%" 2>nul
if errorlevel 1 (
    echo [ERROR] Failed to change to script directory: %SCRIPT_DIR%
    pause
    exit /b 1
)

REM Print banner
echo ================================================================================
echo    NogoMiner Windows Startup Script
echo    Directory: %SCRIPT_DIR%
echo ================================================================================
echo.

REM Check if executable exists
if not exist "%EXE_NAME%" (
    echo [ERROR] Executable not found: %EXE_NAME%
    echo [INFO]  Please ensure %EXE_NAME% is in the same directory as this script.
    pause
    exit /b 1
)
echo [INFO]  Found executable: %EXE_NAME%

REM Check if config file exists
if not exist "%CONFIG_NAME%" (
    echo [WARN]  Config file not found: %CONFIG_NAME%
    echo [INFO]  NogoMiner will use default configuration.
    echo [INFO]  Recommendation: Create %CONFIG_NAME% from configs\config.example.json
) else (
    echo [INFO]  Found config file: %CONFIG_NAME%
)

REM Check if miner is already running
tasklist /fi "imagename eq %EXE_NAME%" 2>nul | find /i "%EXE_NAME%" >nul
if not errorlevel 1 (
    echo [WARN]  NogoMiner is already running!
    echo [INFO]  Please stop the existing instance before starting a new one.
    echo.
    pause
    exit /b 1
)

REM Set environment variables (optional optimizations)
set "GODEBUG=asyncpreemptoff=1"
set "GOMAXPROCS="

REM =============================================================================
REM Detect CPU cores and calculate 90% threads
REM CRITICAL: Use delayed expansion (!var!) inside parenthesized blocks,
REM because %var% expands at parse time (before variables are set).
REM =============================================================================
set "THREADS="

REM Try wmic first (traditional approach)
for /f "tokens=2 delims==" %%i in ('wmic cpu get NumberOfCores /value 2^>nul') do (
    set /a "CPU_CORES=%%i" 2>nul
)

REM If wmic failed, try MSFT_LogicalDisk via PowerShell as fallback
if not defined CPU_CORES (
    for /f "delims=" %%i in ('PowerShell -NoProfile -Command "& { (Get-CimInstance Win32_Processor).NumberOfCores }" 2^>nul') do (
        set /a "CPU_CORES=%%i" 2>nul
    )
)

if defined CPU_CORES (
    REM Calculate 90% threads (ceil division: (n*9+5)/10)
    set /a "THREADS=(CPU_CORES*9+5)/10"

    REM Ensure at least 1 thread
    if !THREADS! LSS 1 set "THREADS=1"

    echo [INFO]  Detected CPU cores: %CPU_CORES%
    echo [INFO]  Using 90%% threads: !THREADS! (!THREADS! of %CPU_CORES%)
    echo [INFO]  Setting GOMAXPROCS=!THREADS!
    set "GOMAXPROCS=!THREADS!"
) else (
    echo [INFO]  Could not detect CPU cores, using config default
)

REM Display startup information
echo.
echo ================================================================================
echo    Starting NogoMiner...
echo    Executable: %SCRIPT_DIR%%EXE_NAME%
echo    Config:    %SCRIPT_DIR%%CONFIG_NAME%
echo    Log:       %SCRIPT_DIR%%LOG_NAME%
echo    Time:      %date% %time%
echo ================================================================================
echo.
echo [INFO]  NogoMiner is running... (Press Ctrl+C to stop)
echo.

REM =============================================================================
REM Launch NogoMiner with threads auto-detection
REM NOTE: Pass -threads flag directly instead of modifying config.json.
REM This avoids JSON corruption from PowerShell serialization.
REM =============================================================================
if defined THREADS (
    if !THREADS! GTR 0 (
        echo [INFO]  Launching NogoMiner with !THREADS! threads...
        "%SCRIPT_DIR%%EXE_NAME%" -threads !THREADS!
    ) else (
        "%SCRIPT_DIR%%EXE_NAME%"
    )
) else (
    "%SCRIPT_DIR%%EXE_NAME%"
)
set "EXIT_CODE=%errorlevel%"

REM Check exit code
if %EXIT_CODE% neq 0 (
    echo.
    echo [ERROR] NogoMiner exited with code: %EXIT_CODE%
    echo [INFO]  Check log file for details: %SCRIPT_DIR%%LOG_NAME%
    if exist "%LOG_NAME%" (
        echo.
        echo [INFO]  Last 10 lines of log:
        PowerShell -NoProfile -Command "Get-Content '%LOG_NAME%' -Tail 10" 2>nul
    )
    echo.
    pause
    exit /b %EXIT_CODE%
)

echo.
echo [INFO]  NogoMiner stopped normally.
echo [INFO]  Exit code: %EXIT_CODE%
echo.
pause
exit /b 0