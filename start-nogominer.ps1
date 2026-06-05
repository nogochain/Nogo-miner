# =============================================================================
# NogoMiner Windows Startup Script (PowerShell)
# Double-click to start NogoMiner with config.json
# Production-grade implementation with error handling and logging
# =============================================================================

[CmdletBinding()]
param(
    [string]$ConfigFile = "config.json",
    [string]$LogFile = "nogominer.log",
    [switch]$Force
)

$ScriptName = $MyInvocation.MyCommand.Name
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$ExeName = "nogominer.exe"
$ExePath = Join-Path $ScriptDir $ExeName
$ConfigPath = Join-Path $ScriptDir $ConfigFile
$LogPath = Join-Path $ScriptDir $LogFile

# Change to script directory
Set-Location -Path $ScriptDir -ErrorAction Stop

# Print banner
Write-Host "================================================================================" -ForegroundColor Cyan
Write-Host "   NogoMiner Windows Startup Script (PowerShell)" -ForegroundColor Cyan
Write-Host "   Directory: $ScriptDir" -ForegroundColor Cyan
Write-Host "================================================================================" -ForegroundColor Cyan
Write-Host ""

# Function: Print formatted message
function Print-Message {
    param(
        [string]$Level,
        [string]$Message
    )
    
    $timestamp = Get-Date -Format "yyyy-MM-dd HH:mm:ss"
    $color = switch ($Level) {
        "INFO"  { "White" }
        "WARN"  { "Yellow" }
        "ERROR" { "Red" }
        "OK"    { "Green" }
        default  { "Gray" }
    }
    
    Write-Host "[$timestamp] [$Level] $Message" -ForegroundColor $color
}

# Check if executable exists
if (-not (Test-Path -Path $ExePath -PathType Leaf)) {
    Print-Message -Level "ERROR" -Message "Executable not found: $ExeName"
    Print-Message -Level "INFO" -Message "Please ensure $ExeName is in the same directory as this script."
    Print-Message -Level "INFO" -Message "Script directory: $ScriptDir"
    Write-Host ""
    Read-Host "Press Enter to exit"
    exit 1
}
Print-Message -Level "OK" -Message "Found executable: $ExeName"

# Check if config file exists
if (-not (Test-Path -Path $ConfigPath -PathType Leaf)) {
    Print-Message -Level "WARN" -Message "Config file not found: $ConfigFile"
    Print-Message -Level "INFO" -Message "NogoMiner will use default configuration."
    Print-Message -Level "INFO" -Message "Recommendation: Copy configs\config.example.json to $ConfigFile"
} else {
    Print-Message -Level "OK" -Message "Found config file: $ConfigFile"
}

# Check if miner is already running
$existingProcess = Get-Process -Name "nogominer" -ErrorAction SilentlyContinue
if ($existingProcess) {
    if (-not $Force) {
        Print-Message -Level "WARN" -Message "NogoMiner is already running (PID: $($existingProcess.Id))"
        Print-Message -Level "INFO" -Message "Use -Force to start another instance, or stop existing instance first."
        Write-Host ""
        Read-Host "Press Enter to exit"
        exit 1
    } else {
        Print-Message -Level "WARN" -Message "Force starting while another instance is running (may cause issues)"
    }
}

# Detect CPU cores for GOMAXPROCS optimization
try {
    $cpuCores = (Get-CimInstance -ClassName Win32_Processor).NumberOfCores
    if ($cpuCores) {
        $env:GOMAXPROCS = $cpuCores
        Print-Message -Level "INFO" -Message "Detected CPU cores: $cpuCores"
        Print-Message -Level "INFO" -Message "Set GOMAXPROCS=$cpuCores"
    }
} catch {
    Print-Message -Level "INFO" -Message "Could not detect CPU cores, using default GOMAXPROCS"
}

# Set environment variables (optional optimizations)
$env:GODEBUG = "asyncpreemptoff=1"

# Display startup information
Write-Host ""
Write-Host "================================================================================" -ForegroundColor Cyan
Write-Host "   Starting NogoMiner..." -ForegroundColor Cyan
Write-Host "   Executable: $ExePath" -ForegroundColor Cyan
Write-Host "   Config:    $ConfigPath" -ForegroundColor Cyan
Write-Host "   Log:       $LogPath" -ForegroundColor Cyan
Write-Host "   Time:      $(Get-Date -Format 'yyyy-MM-dd HH:mm:ss')" -ForegroundColor Cyan
Write-Host "================================================================================" -ForegroundColor Cyan
Write-Host ""

# Start NogoMiner with error handling
try {
    Print-Message -Level "INFO" -Message "Starting NogoMiner..."
    
    # Start process and wait
    $process = Start-Process -FilePath $ExePath -WorkingDirectory $ScriptDir -Wait -NoNewWindow -PassThru
    $exitCode = $process.ExitCode
    
    # Check exit code
    if ($exitCode -ne 0) {
        Write-Host ""
        Print-Message -Level "ERROR" -Message "NogoMiner exited with code: $exitCode"
        Print-Message -Level "INFO" -Message "Check log file for details: $LogPath"
        Write-Host ""
        Read-Host "Press Enter to exit"
        exit $exitCode
    }
    
    Print-Message -Level "OK" -Message "NogoMiner stopped normally."
    exit 0
    
} catch {
    Write-Host ""
    Print-Message -Level "ERROR" -Message "Failed to start NogoMiner: $_"
    Print-Message -Level "INFO" -Message "Check executable path and permissions."
    Write-Host ""
    Read-Host "Press Enter to exit"
    exit 1
}
