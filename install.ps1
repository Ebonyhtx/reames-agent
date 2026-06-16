# Reames Agent — Windows One-Command Installer
# Usage in PowerShell: irm https://raw.githubusercontent.com/Ebonyhtx/reames-agent/main/install.ps1 | iex
#
# 一键完成: Python/Git检查 → 克隆 → venv → pip安装 → PATH → 隔离旧Hermes → 配置向导

param(
    [string]$Branch = "main",
    [string]$InstallDir = "",
    [switch]$Quiet = $false
)

$ErrorActionPreference = "Stop"
$RepoUrl = "https://github.com/Ebonyhtx/reames-agent.git"

function Write-Color($text, $color = "White") { Write-Host $text -ForegroundColor $color }

Write-Color @"
╔══════════════════════════════════════════════╗
║    Reames Agent — 智能安装向导 v0.16.0       ║
╚══════════════════════════════════════════════╝
"@ "Cyan"

# ── Find Python ──
 = 
foreach ( in @("python3", "python")) {
    try {  = Get-Command  -ErrorAction Stop;  = ; break } catch {}
}
if (-not ) {
     = Get-Item ":APPDATA\uv\python\cpython-3.*-windows-x86_64-none\python.exe" -ErrorAction SilentlyContinue | Select-Object -First 1
    if () {  = .FullName }
}
if (-not ) {
     = Get-Item ":LOCALAPPDATA\Programs\Python\Python3*\python.exe" -ErrorAction SilentlyContinue | Select-Object -First 1
    if () {  = .FullName }
}
if (-not ) {
    Write-Color "Python not found. Trying winget install..." "Yellow"
    try { winget install Python.Python.3.12 --silent --accept-package-agreements 2>&1 | Out-Null } catch {}
     = Get-Item ":LOCALAPPDATA\Programs\Python\Python312\python.exe" -ErrorAction SilentlyContinue
    if () {  = .FullName }
}
if (-not ) {
    Write-Color "Python not found. Install from https://python.org (3.10+ required)" "Red"; exit 1
}
 = &  -c "import sys; print(f'{sys.version_info.major}.{sys.version_info.minor}')"
Write-Color "[OK] Python " "Green"
# ── Find Git ──
try { git --version | Out-Null } catch {
    Write-Color "Git not found. Install from https://git-scm.com" "Red"; exit 1
}
Write-Color "[OK] Git" "Green"

# ── Isolate old Hermes ──
$oldWarns = @()
foreach ($loc in @("$env:LOCALAPPDATA\hermes\config.yaml", "$env:USERPROFILE\.hermes\config.yaml")) {
    if (Test-Path $loc) { $oldWarns += $loc }
}
$oldHomeEnv = [Environment]::GetEnvironmentVariable("HERMES_HOME", "User")
if ($oldHomeEnv -and $oldHomeEnv -like "*.hermes*") {
    Write-Color "[WARN] Old HERMES_HOME detected. Reames will ignore it." "Yellow"
    [Environment]::SetEnvironmentVariable("HERMES_HOME", "", "User")
}
if ($oldWarns.Count -gt 0) {
    Write-Color "[OK] Old Hermes config found — Reames uses separate paths, no conflict." "DarkGray"
}

# ── Install path ──
$installPath = if ($InstallDir) { $InstallDir } else { Join-Path $env:LOCALAPPDATA "reames" }
$repoDir = Join-Path $installPath "reames-agent"

# ── Clone or update ──
if (Test-Path $repoDir) {
    Write-Color "Updating existing Reames..." "Cyan"
    Set-Location $repoDir
    git fetch origin $Branch 2>&1 | Out-Null
    git reset --hard origin/$Branch 2>&1 | Out-Null
} else {
    Write-Color "Downloading Reames Agent..." "Cyan"
    New-Item -ItemType Directory -Force -Path $installPath | Out-Null
    git clone --depth 1 --branch $Branch $RepoUrl $repoDir 2>&1
}

# ── Create venv ──
Set-Location $repoDir
if (-not (Test-Path ".venv")) {
    & $pythonCmd -m venv .venv
}
$pip = Join-Path ".venv" "Scripts" "pip.exe"
try { & $pip install --quiet --upgrade pip 2>&1 | Out-Null } catch {}
Write-Color "[OK] Installing dependencies (1-3 minutes)..." "Cyan"
& $pip install --quiet -e . 2>&1 | Out-Null
Write-Color "[OK] Reames Agent installed!" "Green"

# ── Add to PATH ──
$binDir = Join-Path $repoDir ".venv" "Scripts"
$userPath = [Environment]::GetEnvironmentVariable("PATH", "User")
if ($userPath -notlike "*$binDir*") {
    [Environment]::SetEnvironmentVariable("PATH", "$userPath;$binDir", "User")
    $env:PATH = "$env:PATH;$binDir"
    Write-Color "[OK] 'reames' command added to PATH (restart terminal to use)" "Green"
} else {
    Write-Color "[OK] 'reames' already in PATH" "Green"
}

# ── Setup Wizard ──
if (-not $Quiet) {
    Write-Color ""
    $existingKey = [Environment]::GetEnvironmentVariable("DEEPSEEK_API_KEY", "User")
    if ($existingKey) {
        Write-Color "[OK] DEEPSEEK_API_KEY already set" "Green"
    } else {
        Write-Color "Enter your DeepSeek API Key (from https://platform.deepseek.com):" "Cyan"
        Write-Color "(leave blank to skip and set later via 'setx DEEPSEEK_API_KEY sk-xxx')" "DarkGray"
        $key = Read-Host "API Key"
        if ($key) {
            [Environment]::SetEnvironmentVariable("DEEPSEEK_API_KEY", $key, "User")
            $env:DEEPSEEK_API_KEY = $key
        }
    }
    
    # Embedding key for memory vector search
    $existingEmbKey = [Environment]::GetEnvironmentVariable("MEMORY_EMBEDDING_API_KEY", "User")
    if ($existingEmbKey) {
        Write-Color "[OK] MEMORY_EMBEDDING_API_KEY already set" "Green"
    } else {
        Write-Color "Enter your SiliconFlow API Key for memory vector search (optional):" "Cyan"
        Write-Color "(from https://siliconflow.cn — BGE-M3 model, free tier available)" "DarkGray"
        Write-Color "(leave blank to disable vector search — keyword search still works)" "DarkGray"
        $embKey = Read-Host "Embedding Key"
        if ($embKey) {
            [Environment]::SetEnvironmentVariable("MEMORY_EMBEDDING_API_KEY", $embKey, "User")
            $env:MEMORY_EMBEDDING_API_KEY = $embKey
        }
    }
}

Write-Color @"
Done! Restart terminal and type: reames
"@ "Green"
