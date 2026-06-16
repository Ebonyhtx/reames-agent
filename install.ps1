# Reames Agent — Windows One-Command Installer
# Usage: irm https://raw.githubusercontent.com/Ebonyhtx/reames-agent/main/install.ps1 | iex

param([string]$Branch = "main", [string]$InstallDir = "", [switch]$Quiet = $false)
$ErrorActionPreference = "Stop"
$RepoUrl = "https://github.com/Ebonyhtx/reames-agent.git"

Write-Host "Reames Agent Installer v0.16.0" -ForegroundColor Cyan

# Find Python
$pythonCmd = $null
foreach ($cmd in @("python3", "python")) {
    try { $null = Get-Command $cmd -ErrorAction Stop; $pythonCmd = $cmd; break } catch {}
}
if (-not $pythonCmd) {
    Write-Host "Installing Python 3.12 via winget..." -ForegroundColor Yellow
    winget install Python.Python.3.12 --silent --accept-source-agreements --accept-package-agreements
    Write-Host "Done. Restart terminal and re-run this command." -ForegroundColor Green
    exit 0
}
$ver = & $pythonCmd -c "import sys; print(f'{sys.version_info.major}.{sys.version_info.minor}')"
Write-Host "[OK] Python $ver" -ForegroundColor Green

# Find Git
try { git --version | Out-Null } catch {
    Write-Host "Git not found: https://git-scm.com" -ForegroundColor Red; exit 1
}
Write-Host "[OK] Git" -ForegroundColor Green

# Check old Hermes
$oldHome = [Environment]::GetEnvironmentVariable("HERMES_HOME", "User")
if ($oldHome -and $oldHome -like "*.hermes*") {
    Write-Host "[WARN] Old HERMES_HOME ignored" -ForegroundColor Yellow
    [Environment]::SetEnvironmentVariable("HERMES_HOME", "", "User")
}

# Install
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

Set-Location $repoDir
if (-not (Test-Path ".venv")) { & $pythonCmd -m venv .venv }

$pip = Join-Path ".venv" "Scripts" "pip.exe"
try { & $pip install --quiet --upgrade pip 2>&1 | Out-Null } catch {}
Write-Host "Installing dependencies (1-3 min)..." -ForegroundColor Cyan
& $pip install --quiet -e . 2>&1 | Out-Null
Write-Host "[OK] Reames Agent installed!" -ForegroundColor Green

# PATH
$binDir = Join-Path $repoDir ".venv" "Scripts"
$userPath = [Environment]::GetEnvironmentVariable("PATH", "User")
if ($userPath -notlike "*$binDir*") {
    [Environment]::SetEnvironmentVariable("PATH", "$userPath;$binDir", "User")
    $env:PATH = "$env:PATH;$binDir"
    Write-Host "[OK] reames added to PATH" -ForegroundColor Green
}

# Setup Wizard
if (-not $Quiet) {
    $existingKey = [Environment]::GetEnvironmentVariable("DEEPSEEK_API_KEY", "User")
    if (-not $existingKey) {
        Write-Host "Enter DeepSeek API Key (from platform.deepseek.com):" -ForegroundColor Cyan
        $key = Read-Host "API Key"
        if ($key) {
            [Environment]::SetEnvironmentVariable("DEEPSEEK_API_KEY", $key, "User")
            $env:DEEPSEEK_API_KEY = $key
        }
    }
    $embKey = [Environment]::GetEnvironmentVariable("MEMORY_EMBEDDING_API_KEY", "User")
    if (-not $embKey) {
        Write-Host "Enter SiliconFlow API Key for memory (optional):" -ForegroundColor Cyan
        $ek = Read-Host "Embedding Key"
        if ($ek) {
            [Environment]::SetEnvironmentVariable("MEMORY_EMBEDDING_API_KEY", $ek, "User")
        }
    }
}

Write-Host "Done! Restart terminal and type: reames" -ForegroundColor Green
