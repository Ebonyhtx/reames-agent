# Reames Agent installer for Windows PowerShell.
#
# This installer is source-build based until public release artifacts are
# enabled. It installs one Go binary and can optionally install the social
# gateway as a user-level Scheduled Task via:
#   reames-agent gateway install --start-now

param(
    [string]$Repo = $(if ($env:REAMES_AGENT_REPO_URL) { $env:REAMES_AGENT_REPO_URL } else { "https://github.com/Ebonyhtx/reames-agent.git" }),
    [string]$Branch = $(if ($env:REAMES_AGENT_BRANCH) { $env:REAMES_AGENT_BRANCH } else { "main" }),
    [string]$InstallDir = $(if ($env:REAMES_AGENT_INSTALL_DIR) {
        $env:REAMES_AGENT_INSTALL_DIR
    } elseif ($env:LOCALAPPDATA) {
        Join-Path $env:LOCALAPPDATA "ReamesAgent\bin"
    } else {
        Join-Path $HOME ".reames-agent/bin"
    }),
    [switch]$SkipSetup,
    [switch]$Gateway,
    [string]$Channels = "",
    [string]$GatewayDir = "",
    [switch]$DryRun
)

$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"

function Invoke-Step {
    param([scriptblock]$Block, [string]$Description)
    if ($DryRun) {
        Write-Host "+ $Description"
        return
    }
    & $Block
}

function Require-Command {
    param([string]$Name, [string]$Message)
    if (-not (Get-Command $Name -ErrorAction SilentlyContinue)) {
        throw $Message
    }
}

if (-not $DryRun) {
    Require-Command git "git is required"
    Require-Command go "Go 1.25+ is required until release binaries are available"
}

$binPath = Join-Path $InstallDir "reames-agent.exe"
$workDir = Join-Path ([IO.Path]::GetTempPath()) ("reames-agent-install-" + [Guid]::NewGuid().ToString("N"))

Write-Host "Installing Reames Agent"
Write-Host "  repo:   $Repo"
Write-Host "  ref:    $Branch"
Write-Host "  binary: $binPath"

Invoke-Step { Remove-Item -LiteralPath $workDir -Recurse -Force -ErrorAction SilentlyContinue } "remove $workDir"
Invoke-Step { git clone --depth 1 --branch $Branch $Repo $workDir } "git clone --depth 1 --branch $Branch $Repo $workDir"
Invoke-Step { New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null } "mkdir $InstallDir"
Invoke-Step {
    Push-Location $workDir
    try {
        $env:CGO_ENABLED = "0"
        go build -ldflags="-s -w" -o $binPath ./cmd/reames-agent
    } finally {
        Pop-Location
    }
} "go build -o $binPath ./cmd/reames-agent"

$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if (($userPath -split ';') -notcontains $InstallDir) {
    if ($DryRun) {
        Write-Host "+ add $InstallDir to user PATH"
    } else {
        $newPath = if ([string]::IsNullOrWhiteSpace($userPath)) { $InstallDir } else { "$InstallDir;$userPath" }
        [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
        $env:Path = "$InstallDir;$env:Path"
        Write-Host "Added to user PATH: $InstallDir"
    }
}

if (-not $SkipSetup) {
    Invoke-Step { & $binPath setup } "$binPath setup"
}

if ($Gateway) {
    $gatewayArgs = @("gateway", "install", "--start-now")
    if ($Channels.Trim() -ne "") {
        $gatewayArgs += @("--channels", $Channels.Trim())
    }
    if ($GatewayDir.Trim() -ne "") {
        $gatewayArgs += @("--dir", $GatewayDir.Trim())
    }
    if ($DryRun) {
        $gatewayArgs += "--dry-run"
    }
    Invoke-Step { & $binPath @gatewayArgs } "$binPath $($gatewayArgs -join ' ')"
}

Invoke-Step { Remove-Item -LiteralPath $workDir -Recurse -Force -ErrorAction SilentlyContinue } "remove $workDir"
Write-Host "Reames Agent install complete."
