# Reames Agent — Windows One-Command Installer
# Usage: irm https://raw.githubusercontent.com/Ebonyhtx/reames-agent/main/install.ps1 | iex

param([string]$Branch = "main", [string]$InstallDir = "", [switch]$Quiet = $false)
$ErrorActionPreference = "Stop"
$RepoUrl = "https://github.com/Ebonyhtx/reames-agent.git"

Write-Host "Reames Agent Installer v0.16.0" -ForegroundColor Cyan

# ============================================================
# Find Python: PATH → uv → System → winget
# ============================================================
$pythonCmd = $null

# 1. PATH
foreach ($cmd in @("python3", "python")) {
    try { $null = Get-Command $cmd -ErrorAction Stop; $pythonCmd = $cmd; break } catch {}
}

# 2. uv Python
if (-not $pythonCmd) {
    $uvPaths = @(
        "$env:APPDATA\uv\python\cpython-3.*-windows-x86_64-none\python.exe",
        "$env:LOCALAPPDATA\uv\python\cpython-3.*-windows-x86_64-none\python.exe"
    )
    foreach ($pattern in $uvPaths) {
        $found = Get-Item $pattern -ErrorAction SilentlyContinue | Select-Object -First 1
        if ($found) { $pythonCmd = $found.FullName; break }
    }
}

# 3. System Python
if (-not $pythonCmd) {
    $sysPaths = @(
        "$env:LOCALAPPDATA\Programs\Python\Python3*\python.exe",
        "C:\Python3*\python.exe",
        "$env:ProgramFiles\Python3*\python.exe"
    )
    foreach ($pattern in $sysPaths) {
        $found = Get-Item $pattern -ErrorAction SilentlyContinue | Select-Object -First 1
        if ($found) { $pythonCmd = $found.FullName; break }
    }
}

# 4. winget auto-install
if (-not $pythonCmd) {
    Write-Host "Python not found. Installing Python 3.12 via winget..." -ForegroundColor Yellow
    try {
        winget install Python.Python.3.12 --silent --accept-source-agreements --accept-package-agreements 2>&1 | Out-Null
        $sysPy = Get-Item "$env:LOCALAPPDATA\Programs\Python\Python312\python.exe" -ErrorAction SilentlyContinue
        if ($sysPy) { $pythonCmd = $sysPy.FullName }
    } catch {
        Write-Host "Winget not available. Install Python manually: https://python.org" -ForegroundColor Red
        exit 1
    }
}

if (-not $pythonCmd) {
    Write-Host "Python not found. Install from https://python.org (3.10+)" -ForegroundColor Red; exit 1
}
$ver = & $pythonCmd -c "import sys; print(f'{sys.version_info.major}.{sys.version_info.minor}')"
Write-Host "[OK] Python $ver" -ForegroundColor Green

# ============================================================
# Find Git: PATH → winget
# ============================================================
try { git --version | Out-Null } catch {
    Write-Host "Git not found. Installing via winget..." -ForegroundColor Yellow
    try {
        winget install Git.Git --silent --accept-source-agreements --accept-package-agreements 2>&1 | Out-Null
        Write-Host "Done. Restart terminal and re-run." -ForegroundColor Green; exit 0
    } catch {
        Write-Host "Install Git manually: https://git-scm.com" -ForegroundColor Red; exit 1
    }
}
Write-Host "[OK] Git" -ForegroundColor Green

# ============================================================
# Check old Hermes
# ============================================================
$oldHome = [Environment]::GetEnvironmentVariable("HERMES_HOME", "User")
if ($oldHome -and $oldHome -like "*.hermes*") {
    Write-Host "[WARN] Old HERMES_HOME found. Reames will ignore it." -ForegroundColor Yellow
    [Environment]::SetEnvironmentVariable("HERMES_HOME", "", "User")
}

# ============================================================
# Clone / Update
# ============================================================
$installPath = if ($InstallDir) { $InstallDir } else { Join-Path $env:LOCALAPPDATA "reames" }
$repoDir = Join-Path $installPath "reames-agent"

if (Test-Path $repoDir) {
    Write-Host "Updating Reames..." -ForegroundColor Cyan
    Set-Location $repoDir
    git fetch origin $Branch | Out-Null
    git reset --hard origin/$Branch | Out-Null
} else {
    Write-Host "Downloading Reames..." -ForegroundColor Cyan
    New-Item -ItemType Directory -Force -Path $installPath | Out-Null
    git clone --depth 1 --branch $Branch $RepoUrl $repoDir
}

# ============================================================
# venv + pip install
# ============================================================
Set-Location $repoDir
if (-not (Test-Path ".venv")) { & $pythonCmd -m venv .venv }

$pip = Join-Path (Join-Path ".venv" "Scripts") "pip.exe"
try { & $pip install --quiet --upgrade pip 2>&1 | Out-Null } catch {}
Write-Host "Installing dependencies (1-3 min)..." -ForegroundColor Cyan
& $pip install --quiet -e . 2>&1 | Out-Null
Write-Host "[OK] Reames Agent installed!" -ForegroundColor Green

# ============================================================
# PATH
# ============================================================
$binDir = Join-Path (Join-Path $repoDir ".venv") "Scripts"
$userPath = [Environment]::GetEnvironmentVariable("PATH", "User")
if ($userPath -notlike "*$binDir*") {
    [Environment]::SetEnvironmentVariable("PATH", "$userPath;$binDir", "User")
    $env:PATH = "$env:PATH;$binDir"
    Write-Host "[OK] 'reames' added to PATH (restart terminal)" -ForegroundColor Green
}

# ============================================================
# Setup Wizard
# ============================================================
if (-not $Quiet) {
    $existingKey = [Environment]::GetEnvironmentVariable("DEEPSEEK_API_KEY", "User")
    if (-not $existingKey) {
        Write-Host "`nEnter DeepSeek API Key (from platform.deepseek.com):" -ForegroundColor Cyan
        Write-Host "(leave blank to skip)" -ForegroundColor DarkGray
        $key = Read-Host "API Key"
        if ($key) {
            [Environment]::SetEnvironmentVariable("DEEPSEEK_API_KEY", $key, "User")
            $env:DEEPSEEK_API_KEY = $key
        }
    }
    $embKey = [Environment]::GetEnvironmentVariable("MEMORY_EMBEDDING_API_KEY", "User")
    if (-not $embKey) {
        Write-Host "`nEnter SiliconFlow API Key for memory (optional):" -ForegroundColor Cyan
        Write-Host "(leave blank to disable vector search)" -ForegroundColor DarkGray
        $ek = Read-Host "Embedding Key"
        if ($ek) {
            [Environment]::SetEnvironmentVariable("MEMORY_EMBEDDING_API_KEY", $ek, "User")
        }
    }
}

Write-Host "`nDone! Restart terminal and type: reames" -ForegroundColor Green
