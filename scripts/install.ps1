# Reames Agent installer for Windows PowerShell.
#
# This installer defaults to source-build mode until stable public release
# artifacts are enabled. It can also install an explicit GitHub Release artifact
# with SHA256 verification when -BinarySource release -Version vX.Y.Z is used.
# It installs one Go binary and can optionally install the social gateway as a
# user-level Scheduled Task via:
#   reames-agent gateway install --start-now

param(
    [string]$Repo = $(if ($env:REAMES_AGENT_REPO_URL) { $env:REAMES_AGENT_REPO_URL } else { "https://github.com/Ebonyhtx/reames-agent.git" }),
    [string]$Branch = $(if ($env:REAMES_AGENT_BRANCH) { $env:REAMES_AGENT_BRANCH } else { "main" }),
    [ValidateSet("source", "release")]
    [string]$BinarySource = $(if ($env:REAMES_AGENT_BINARY_SOURCE) { $env:REAMES_AGENT_BINARY_SOURCE } else { "source" }),
    [string]$Version = $(if ($env:REAMES_AGENT_VERSION) { $env:REAMES_AGENT_VERSION } else { "" }),
    [string]$ReleaseBaseUrl = $(if ($env:REAMES_AGENT_RELEASE_BASE_URL) { $env:REAMES_AGENT_RELEASE_BASE_URL } else { "https://github.com/Ebonyhtx/reames-agent/releases/download" }),
    [string]$InstallDir = $(if ($env:REAMES_AGENT_INSTALL_DIR) {
        $env:REAMES_AGENT_INSTALL_DIR
    } elseif ($env:LOCALAPPDATA) {
        Join-Path $env:LOCALAPPDATA "ReamesAgent\bin"
    } else {
        Join-Path $HOME ".reames-agent/bin"
    }),
    [string]$AgentHome = $(if ($env:REAMES_AGENT_HOME) {
        $env:REAMES_AGENT_HOME
    } elseif ($env:APPDATA) {
        Join-Path $env:APPDATA "reames-agent"
    } else {
        Join-Path $HOME ".reames-agent"
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

function Get-ReamesEnvPath {
    param([string]$HomePath)
    $trimmed = $HomePath.Trim()
    if ([string]::IsNullOrWhiteSpace($trimmed)) {
        return ".env"
    }
    if ($trimmed.Contains("\") -and -not $trimmed.Contains("/")) {
        return Join-Path $trimmed ".env"
    }
    return ($trimmed.TrimEnd("/") + "/.env")
}

function Get-ReleaseTarget {
    $processorArch = if ($env:PROCESSOR_ARCHITECTURE) {
        $env:PROCESSOR_ARCHITECTURE
    } elseif ($DryRun) {
        "AMD64"
    } else {
        throw "PROCESSOR_ARCHITECTURE is not set; install.ps1 release mode is intended for Windows"
    }
    $arch = switch ($processorArch) {
        "AMD64" { "amd64"; break }
        "ARM64" { "arm64"; break }
        default { throw "unsupported architecture for release artifact: $processorArch" }
    }
    return "windows", $arch
}

function Install-FromSource {
    if (-not $DryRun) {
        Require-Command git "git is required"
        Require-Command go "Go 1.25+ is required for source installs; use -BinarySource release -Version vX.Y.Z after stable release artifacts are available"
    }
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
}

function Install-FromRelease {
    if ([string]::IsNullOrWhiteSpace($Version)) {
        throw "-BinarySource release requires -Version vMAJOR.MINOR.PATCH"
    }
    $target = Get-ReleaseTarget
    $goos = $target[0]
    $goarch = $target[1]
    $asset = "reames-agent-$goos-$goarch.zip"
    $releaseUrl = "$ReleaseBaseUrl/$Version"
    $archive = Join-Path $workDir $asset
    $checksum = Join-Path $workDir "SHA256SUMS"
    $extractDir = Join-Path $workDir "release"

    Invoke-Step { New-Item -ItemType Directory -Force -Path $workDir, $InstallDir, $extractDir | Out-Null } "mkdir $workDir $InstallDir $extractDir"
    Invoke-Step { Invoke-WebRequest -UseBasicParsing -Uri "$releaseUrl/$asset" -OutFile $archive } "download $releaseUrl/$asset"
    Invoke-Step { Invoke-WebRequest -UseBasicParsing -Uri "$releaseUrl/SHA256SUMS" -OutFile $checksum } "download $releaseUrl/SHA256SUMS"

    if ($DryRun) {
        Write-Host "+ verify SHA256SUMS contains $asset and matches downloaded archive"
    } else {
        $line = Get-Content -LiteralPath $checksum | Where-Object { $_ -match "(\s|^)$([Regex]::Escape($asset))$" } | Select-Object -First 1
        if (-not $line) {
            throw "SHA256SUMS does not contain $asset"
        }
        $expected = ($line -split '\s+')[0].ToLowerInvariant()
        $actual = (Get-FileHash -Algorithm SHA256 -LiteralPath $archive).Hash.ToLowerInvariant()
        if ($actual -ne $expected) {
            throw "checksum mismatch for ${asset}: got $actual, want $expected"
        }
    }

    Invoke-Step { Expand-Archive -LiteralPath $archive -DestinationPath $extractDir -Force } "expand $archive"
    Invoke-Step { Copy-Item -LiteralPath (Join-Path $extractDir "reames-agent.exe") -Destination $binPath -Force } "install $binPath"
}

$binPath = Join-Path $InstallDir "reames-agent.exe"
$workDir = Join-Path ([IO.Path]::GetTempPath()) ("reames-agent-install-" + [Guid]::NewGuid().ToString("N"))

Write-Host "Installing Reames Agent"
Write-Host "  binary mode: $BinarySource"
Write-Host "  repo:        $Repo"
Write-Host "  ref:         $Branch"
if ($BinarySource -eq "release") {
    Write-Host "  release:     $ReleaseBaseUrl/$Version"
}
Write-Host "  binary:      $binPath"
Write-Host "  home:        $AgentHome"

Invoke-Step { Remove-Item -LiteralPath $workDir -Recurse -Force -ErrorAction SilentlyContinue } "remove $workDir"

if ($BinarySource -eq "release") {
    Install-FromRelease
} else {
    Install-FromSource
}

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
    Invoke-Step {
        $oldHome = $env:REAMES_AGENT_HOME
        try {
            $env:REAMES_AGENT_HOME = $AgentHome
            & $binPath setup
        } finally {
            $env:REAMES_AGENT_HOME = $oldHome
        }
    } "REAMES_AGENT_HOME=$AgentHome $binPath setup"
}

if ($Gateway) {
    Write-Host "Gateway credential source: $(Get-ReamesEnvPath $AgentHome)"
    Write-Host "Gateway service definitions pin REAMES_AGENT_HOME and do not embed secret values."
    $gatewayArgs = @("gateway", "install", "--start-now", "--home", $AgentHome)
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
