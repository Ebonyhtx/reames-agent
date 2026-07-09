param(
    [switch]$SkipDesktop,
    [switch]$SkipFrontendHint
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$RepoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
$env:GOPROXY = if ($env:GOPROXY) { $env:GOPROXY } else { "https://goproxy.cn,direct" }
$env:GOSUMDB = if ($env:GOSUMDB) { $env:GOSUMDB } else { "sum.golang.google.cn" }

function Invoke-Native {
    param(
        [Parameter(Mandatory = $true)][string]$FilePath,
        [Parameter(ValueFromRemainingArguments = $true)][string[]]$Arguments
    )

    & $FilePath @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "Command failed with exit code ${LASTEXITCODE}: $FilePath $($Arguments -join ' ')"
    }
}

function Invoke-Step {
    param(
        [Parameter(Mandatory = $true)][string]$Name,
        [Parameter(Mandatory = $true)][scriptblock]$Script
    )

    Write-Host ""
    Write-Host "==> $Name" -ForegroundColor Cyan
    & $Script
    }

Push-Location $RepoRoot
try {
    Invoke-Step "Build CLI binary" {
        Invoke-Native -FilePath "go" -Arguments @("build", "-o", (Join-Path $env:TEMP "reames-agent-check.exe"), "./cmd/reames-agent")
    }

    Invoke-Step "Provider/agent cache-sensitive tests" {
        Invoke-Native -FilePath "go" -Arguments @("test", "./internal/provider/openai", "./internal/agent", "-run", "Test(Normalise|Normalize|Usage|Cache|SessionCache|SetSession|ReleaseCache|PlanModeDoesNotMutateSystemOrTools)", "-count=1")
    }

    Invoke-Step "Public and deployment contract checks" {
        Invoke-Native -FilePath "python" -Arguments @("scripts/check_public_readiness.py")
        Invoke-Native -FilePath "python" -Arguments @("scripts/check_deploy_contracts.py")
        Invoke-Native -FilePath "python" -Arguments @("-m", "unittest", "scripts.test_check_upstreams", "-v")
        Invoke-Native -FilePath "node" -Arguments @("scripts/test_upstream_watch_issue.mjs")
    }

    Invoke-Step "Root Go baseline package set" {
        Invoke-Native -FilePath "go" -Arguments @(
            "test",
            "./internal/crypto/...",
            "./internal/trust/...",
            "./internal/cron/...",
            "./internal/board/...",
            "./internal/pluginpkg/...",
            "./internal/config/...",
            "./internal/agent/...",
            "./internal/tool/builtin/...",
            "./internal/provider/...",
            "./internal/hook/...",
            "./internal/skill/...",
            "./internal/lsp/...",
            "-count=1"
        )
    }

    if (-not $SkipDesktop) {
        Invoke-Step "Desktop nested-module critical baseline" {
            Push-Location (Join-Path $RepoRoot "desktop")
            try {
                Invoke-Native -FilePath "go" -Arguments @("test", ".", "-run", "TestWorkspaceChangesGitStatus|TestWorkspaceChangesGitStatusFromRepoSubdirectory|TestWorkspaceChangesUntrackedDirectoryListsFiles|TestWorkspaceChangesGitBranchDetachedHead|TestParseGitStatusPorcelainZ|TestHeartbeatConfigPathUsesReamesAgentUserStateDir", "-count=1")
            }
            finally {
                Pop-Location
            }
        }
    }

    if (-not $SkipFrontendHint) {
        $frontend = Join-Path $RepoRoot "desktop\frontend"
        if (Test-Path $frontend) {
            $nodeModules = Join-Path $frontend "node_modules"
            if (Test-Path $nodeModules) {
                Invoke-Step "Desktop frontend build" {
                    Push-Location $frontend
                    try {
                        Invoke-Native -FilePath "corepack" -Arguments @("pnpm", "build")
                    }
                    finally {
                        Pop-Location
                    }
                }
            }
            else {
                Write-Host ""
                Write-Host "Core baseline passed, but desktop frontend was not verified because node_modules is missing." -ForegroundColor Yellow
                Write-Host "Run: cd desktop\frontend; corepack pnpm install --frozen-lockfile"
            }
        }
    }

    Write-Host ""
    Write-Host "Baseline verification passed." -ForegroundColor Green
}
finally {
    Pop-Location
}
