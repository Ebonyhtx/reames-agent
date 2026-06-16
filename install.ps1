# Reames Agent — Windows One-Command Installer
# Usage: irm https://raw.githubusercontent.com/Ebonyhtx/reames-agent/main/install.ps1 | iex

param([string]$Branch = "main", [string]$InstallDir = "", [switch]$Quiet = $false)
$ErrorActionPreference = "Stop"
$RepoUrl = "https://github.com/Ebonyhtx/reames-agent.git"

Write-Host "Reames Agent Installer v0.16.0" -ForegroundColor Cyan

# Helper: join path components (PS 5.1 compatible)
function Join-PathSafe {
    param([string]$Parent, [string]$Child)
    return [System.IO.Path]::Combine($Parent, $Child)
}

# Helper: test if a Python can create virtual environments
function Test-PythonUsable {
    param([string]$PythonPath)
    try {
        $result = & $PythonPath -c "import venv; print('ok')" 2>&1
        return $result -eq "ok"
    } catch {
        return $false
    }
}

# ============================================================
# Find Python: standard install → uv → system → winget
# Skip embedded/WindowsApp Python (can't create venv)
# ============================================================
$pythonCmd = $null

# 1. Check PATH for a usable Python
foreach ($cmd in @("python3", "python")) {
    try {
        $exe = (Get-Command $cmd -ErrorAction Stop).Source
        # Skip Windows Store stub (0-byte launcher)
        if ($exe -like "*WindowsApps*") { continue }
        # Skip LibreOffice/embedded Python
        if ($exe -like "*LibreOffice*") { continue }
        if (Test-PythonUsable $exe) {
            $pythonCmd = $exe
            break
        }
    } catch { continue }
}

# 2. uv-managed Python
if (-not $pythonCmd) {
    $uvPatterns = @(
        "$env:APPDATA\uv\python\cpython-3.*-windows-x86_64-none\python.exe",
        "$env:LOCALAPPDATA\uv\python\cpython-3.*-windows-x86_64-none\python.exe"
    )
    foreach ($pattern in $uvPatterns) {
        $found = Get-Item $pattern -ErrorAction SilentlyContinue | Select-Object -First 1
        if ($found -and (Test-PythonUsable $found.FullName)) {
            $pythonCmd = $found.FullName
            break
        }
    }
}

# 3. System-installed Python
if (-not $pythonCmd) {
    $sysPatterns = @(
        "$env:LOCALAPPDATA\Programs\Python\Python3*\python.exe",
        "C:\Python3*\python.exe",
        "$env:ProgramFiles\Python3*\python.exe"
    )
    foreach ($pattern in $sysPatterns) {
        $found = Get-Item $pattern -ErrorAction SilentlyContinue | Select-Object -First 1
        if ($found -and (Test-PythonUsable $found.FullName)) {
            $pythonCmd = $found.FullName
            break
        }
    }
}

# 4. winget auto-install
if (-not $pythonCmd) {
    Write-Host "No usable Python found. Installing Python 3.12 via winget..." -ForegroundColor Yellow
    try {
        winget install Python.Python.3.12 --silent --accept-source-agreements --accept-package-agreements 2>&1 | Out-Null
        $newPy = Get-Item "$env:LOCALAPPDATA\Programs\Python\Python312\python.exe" -ErrorAction SilentlyContinue
        if ($newPy) {
            $pythonCmd = $newPy.FullName
            Write-Host "[OK] Python 3.12 installed" -ForegroundColor Green
        } else {
            Write-Host "[ERROR] Python installed but not found. Restart terminal and re-run." -ForegroundColor Red
            exit 1
        }
    } catch {
        Write-Host "[ERROR] Winget unavailable. Install Python from https://python.org (3.10+) then re-run." -ForegroundColor Red
        exit 1
    }
}

if (-not $pythonCmd) {
    Write-Host "[ERROR] No usable Python found." -ForegroundColor Red
    Write-Host "Install Python 3.10+ from https://python.org then re-run." -ForegroundColor Yellow
    exit 1
}

$ver = & $pythonCmd -c "import sys; print(f'{sys.version_info.major}.{sys.version_info.minor}')"
Write-Host "[OK] Python $ver ($pythonCmd)" -ForegroundColor Green

# ============================================================
# Find Git: PATH → winget (refresh PATH after install)
# ============================================================
function Find-Git {
    try { $null = git --version; return $true } catch { return $false }
}

if (-not (Find-Git)) {
    Write-Host "Git not found. Installing via winget..." -ForegroundColor Yellow
    try {
        winget install Git.Git --silent --accept-source-agreements --accept-package-agreements 2>&1 | Out-Null
        # Refresh PATH to pick up newly installed Git
        $env:Path = [Environment]::GetEnvironmentVariable("PATH", "Machine") + ";" + [Environment]::GetEnvironmentVariable("PATH", "User")
        if (-not (Find-Git)) {
            Write-Host "[ERROR] Git installed but not in PATH. Restart terminal and re-run." -ForegroundColor Red
            exit 1
        }
    } catch {
        Write-Host "[ERROR] Winget unavailable. Install Git from https://git-scm.com then re-run." -ForegroundColor Red
        exit 1
    }
}
Write-Host "[OK] Git" -ForegroundColor Green

# ============================================================
# Clean up old Hermes environment variable
# ============================================================
$oldHome = [Environment]::GetEnvironmentVariable("HERMES_HOME", "User")
if ($oldHome -and $oldHome -like "*.hermes*") {
    Write-Host "[WARN] Old HERMES_HOME found. Reames will ignore it." -ForegroundColor Yellow
    [Environment]::SetEnvironmentVariable("HERMES_HOME", "", "User")
}

# ============================================================
# Clone / Update
# ============================================================
$installPath = if ($InstallDir) { $InstallDir } else { Join-PathSafe $env:LOCALAPPDATA "reames" }
$repoDir = Join-PathSafe $installPath "reames-agent"

if (Test-Path $repoDir) {
    Write-Host "Updating Reames..." -ForegroundColor Cyan
    Set-Location $repoDir
    git fetch origin $Branch 2>&1 | Out-Null
    git reset --hard origin/$Branch 2>&1 | Out-Null
} else {
    Write-Host "Downloading Reames..." -ForegroundColor Cyan
    New-Item -ItemType Directory -Force -Path $installPath | Out-Null
    git clone --depth 1 --branch $Branch $RepoUrl $repoDir
}

# ============================================================
# venv + pip install
# ============================================================
Set-Location $repoDir

Write-Host "Setting up Python virtual environment..." -ForegroundColor Cyan
& $pythonCmd -m venv .venv
if (-not (Test-Path ".venv")) {
    Write-Host "[ERROR] Failed to create virtual environment." -ForegroundColor Red
    Write-Host "  Python: $pythonCmd" -ForegroundColor Yellow
    Write-Host "  Try installing a standard Python from https://python.org" -ForegroundColor Yellow
    exit 1
}

$pipPath = Join-PathSafe (Join-PathSafe ".venv" "Scripts") "pip.exe"
if (-not (Test-Path $pipPath)) {
    Write-Host "[ERROR] pip not found in virtual environment." -ForegroundColor Red
    exit 1
}

try { & $pipPath install --quiet --upgrade pip 2>&1 | Out-Null } catch {}
Write-Host "Installing dependencies (1-3 min)..." -ForegroundColor Cyan
& $pipPath install --quiet -e .
if ($LASTEXITCODE -ne 0) {
    Write-Host "[ERROR] pip install failed. Check network or run manually:" -ForegroundColor Red
    Write-Host "  cd $repoDir && .venv\Scripts\pip install -e ." -ForegroundColor Yellow
    exit 1
}
Write-Host "[OK] Reames Agent installed!" -ForegroundColor Green

# ============================================================
# PATH (user-level)
# ============================================================
$binDir = Join-PathSafe $repoDir ".venv\Scripts"
try {
    $userPath = [Environment]::GetEnvironmentVariable("PATH", "User")
    if ($userPath -notlike "*$binDir*") {
        [Environment]::SetEnvironmentVariable("PATH", "$userPath;$binDir", "User")
        $env:PATH = "$env:PATH;$binDir"
        Write-Host "[OK] 'reames' added to PATH (restart terminal)" -ForegroundColor Green
    } else {
        Write-Host "[OK] 'reames' already in PATH" -ForegroundColor Green
    }
} catch {
    Write-Host "[WARN] PATH write failed. Add this to your PATH manually:" -ForegroundColor Yellow
    Write-Host "  $binDir" -ForegroundColor White
}

# ============================================================
# Setup Wizard (DeepSeek API Key prompt)
# ============================================================
if (-not $Quiet) {
    $existingKey = [Environment]::GetEnvironmentVariable("DEEPSEEK_API_KEY", "User")
    if (-not $existingKey) {
        Write-Host "`nEnter DeepSeek API Key (from platform.deepseek.com):" -ForegroundColor Cyan
        Write-Host "(leave blank to skip)" -ForegroundColor DarkGray
        $key = Read-Host "API Key"
        if ($key) {
            [Environment]::SetEnvironmentVariable("DEEPSEEK_API_KEY", $key, "User")
            Write-Host "[OK] DEEPSEEK_API_KEY set" -ForegroundColor Green
        }
    }
}

Write-Host "`nReames Agent is ready!" -ForegroundColor Green
Write-Host "Run: reames --tui" -ForegroundColor Cyan
