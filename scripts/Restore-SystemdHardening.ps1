#Requires -Version 5.1
<#
.SYNOPSIS
    Restores systemd security hardening for samverk services on CT 202.

.DESCRIPTION
    Syncs samverk-serve.service and samverk-dispatch.service from the local
    repo to CT 202, adds ReadWritePaths for the claude-cli OAuth credential
    directory, reloads systemd, restarts both services, and verifies they
    come up healthy.

.PARAMETER Host
    CT 202 SSH target. Default: root@192.168.1.162

.PARAMETER RepoRoot
    Local samverk repo root. Default: D:\DevSpace\Samverk

.PARAMETER DryRun
    Show what would be changed without applying anything.

.EXAMPLE
    .\Restore-SystemdHardening.ps1
    .\Restore-SystemdHardening.ps1 -DryRun
    .\Restore-SystemdHardening.ps1 -Host root@192.168.1.162
#>
param(
    [string]$RemoteHost = "root@192.168.1.162",
    [string]$RepoRoot   = "D:\DevSpace\Samverk",
    [switch]$DryRun
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

# ── Helpers ──────────────────────────────────────────────────────────────────

function Write-Step  { param($msg) Write-Host "`n==> $msg" -ForegroundColor Cyan }
function Write-Ok    { param($msg) Write-Host "    OK  $msg" -ForegroundColor Green }
function Write-Warn  { param($msg) Write-Host "    !!  $msg" -ForegroundColor Yellow }
function Write-Fail  { param($msg) Write-Host "    ERR $msg" -ForegroundColor Red }

function Invoke-SSH {
    param([string]$Cmd)
    $result = & ssh $RemoteHost $Cmd 2>&1
    if ($LASTEXITCODE -ne 0) {
        throw "SSH command failed (exit $LASTEXITCODE): $Cmd`n$result"
    }
    return $result
}

function Copy-ToRemote {
    param([string]$LocalPath, [string]$RemotePath)
    & scp $LocalPath "${RemoteHost}:${RemotePath}" 2>&1 | Out-Null
    if ($LASTEXITCODE -ne 0) {
        throw "scp failed: $LocalPath -> $RemotePath"
    }
}

# ── Validate local repo ───────────────────────────────────────────────────────

Write-Step "Validating local repo at $RepoRoot"

$serveService    = Join-Path $RepoRoot "deploy\samverk-serve.service"
$dispatchService = Join-Path $RepoRoot "deploy\samverk-dispatch.service"

foreach ($f in @($serveService, $dispatchService)) {
    if (-not (Test-Path $f)) {
        Write-Fail "Missing: $f"
        exit 1
    }
    Write-Ok "Found: $f"
}

# ── Patch dispatch service to add .claude ReadWritePath ──────────────────────
# The repo service file has ReadWritePaths=/var/lib/samverk
# claude-cli OAuth credentials live at /var/lib/samverk/.claude
# We add a second ReadWritePaths line for that path.

Write-Step "Preparing patched service files"

$tmpDir          = Join-Path $env:TEMP "samverk-hardening"
New-Item -ItemType Directory -Force -Path $tmpDir | Out-Null

$patchedServe    = Join-Path $tmpDir "samverk-serve.service"
$patchedDispatch = Join-Path $tmpDir "samverk-dispatch.service"

# serve: copy as-is (no claude-cli, no .claude path needed)
Copy-Item $serveService $patchedServe

# dispatch: ensure ReadWritePaths=/var/lib/samverk/.claude is present
$dispatchContent = Get-Content $dispatchService -Raw

if ($dispatchContent -notmatch "ReadWritePaths=/var/lib/samverk/\.claude") {
    $dispatchContent = $dispatchContent -replace `
        "ReadWritePaths=/var/lib/samverk\b", `
        "ReadWritePaths=/var/lib/samverk`nReadWritePaths=/var/lib/samverk/.claude"
    Write-Ok "Added ReadWritePaths=/var/lib/samverk/.claude to dispatch service"
} else {
    Write-Ok "dispatch service already has ReadWritePaths for .claude"
}

Set-Content -Path $patchedDispatch -Value $dispatchContent -NoNewline

# Show diff if DryRun
if ($DryRun) {
    Write-Step "[DRY RUN] samverk-dispatch.service diff"
    $orig    = Get-Content $dispatchService
    $patched = Get-Content $patchedDispatch
    $diff    = Compare-Object $orig $patched
    if ($diff) {
        $diff | ForEach-Object {
            $prefix = if ($_.SideIndicator -eq "=>") { "+ " } else { "- " }
            $color  = if ($_.SideIndicator -eq "=>") { "Green" } else { "Red" }
            Write-Host "    $prefix$($_.InputObject)" -ForegroundColor $color
        }
    } else {
        Write-Warn "No changes to dispatch service"
    }
    Write-Warn "DryRun mode — nothing applied to CT 202"
    exit 0
}

# ── Check SSH connectivity ────────────────────────────────────────────────────

Write-Step "Verifying SSH connectivity to $RemoteHost"
$uptime = Invoke-SSH "uptime"
Write-Ok "CT 202 reachable: $($uptime.Trim())"

# ── Backup existing service files on CT 202 ──────────────────────────────────

Write-Step "Backing up existing service files on CT 202"
$timestamp = Get-Date -Format "yyyyMMdd-HHmmss"

foreach ($svc in @("samverk-serve.service", "samverk-dispatch.service")) {
    $remote = "/etc/systemd/system/$svc"
    $backup = "/etc/systemd/system/${svc}.bak-${timestamp}"
    Invoke-SSH "cp $remote $backup" | Out-Null
    Write-Ok "Backed up $remote -> $backup"
}

# ── Deploy patched service files ─────────────────────────────────────────────

Write-Step "Deploying service files to CT 202"
Copy-ToRemote $patchedServe    "/etc/systemd/system/samverk-serve.service"
Write-Ok "Deployed samverk-serve.service"
Copy-ToRemote $patchedDispatch "/etc/systemd/system/samverk-dispatch.service"
Write-Ok "Deployed samverk-dispatch.service"

# ── Verify .claude directory exists and has correct ownership ─────────────────

Write-Step "Verifying /var/lib/samverk/.claude on CT 202"
try {
    $claudeDir = Invoke-SSH "stat -c '%U:%G %a' /var/lib/samverk/.claude"
    Write-Ok ".claude exists: $claudeDir"
} catch {
    Write-Warn ".claude directory missing — creating it"
    Invoke-SSH "mkdir -p /var/lib/samverk/.claude && chown samverk:samverk /var/lib/samverk/.claude && chmod 700 /var/lib/samverk/.claude" | Out-Null
    Write-Ok "Created /var/lib/samverk/.claude (samverk:samverk 700)"
}

# ── Reload systemd and restart services ──────────────────────────────────────

Write-Step "Reloading systemd daemon"
Invoke-SSH "systemctl daemon-reload" | Out-Null
Write-Ok "systemd daemon reloaded"

Write-Step "Restarting samverk-serve"
Invoke-SSH "systemctl restart samverk-serve" | Out-Null
Start-Sleep -Seconds 3
$serveStatus = Invoke-SSH "systemctl is-active samverk-serve"
if ($serveStatus.Trim() -eq "active") {
    Write-Ok "samverk-serve is active"
} else {
    Write-Fail "samverk-serve failed to start: $serveStatus"
    Write-Warn (Invoke-SSH "journalctl -u samverk-serve -n 20 --no-pager")
    exit 1
}

Write-Step "Restarting samverk-dispatch"
Invoke-SSH "systemctl restart samverk-dispatch" | Out-Null
Start-Sleep -Seconds 5
$dispatchStatus = Invoke-SSH "systemctl is-active samverk-dispatch"
if ($dispatchStatus.Trim() -eq "active") {
    Write-Ok "samverk-dispatch is active"
} else {
    Write-Fail "samverk-dispatch failed to start: $dispatchStatus"
    Write-Warn (Invoke-SSH "journalctl -u samverk-dispatch -n 20 --no-pager")
    exit 1
}

# ── Verify healthz and agent pool ─────────────────────────────────────────────

Write-Step "Verifying HTTP healthz endpoint"
try {
    $health = Invoke-SSH "curl -sf http://localhost:8080/healthz"
    Write-Ok "healthz: $($health.Trim())"
} catch {
    Write-Warn "healthz check failed — serve may still be starting"
}

Write-Step "Checking dispatch log for agent pool startup"
$dispatchLog = Invoke-SSH "journalctl -u samverk-dispatch -n 15 --no-pager"
Write-Host ""
$dispatchLog | ForEach-Object { Write-Host "    $_" }

if ($dispatchLog -match "agent pool started") {
    Write-Ok "Agent pool confirmed started"
} else {
    Write-Warn "agent pool started line not found yet — check logs manually"
}

# ── Cleanup temp files ────────────────────────────────────────────────────────

Remove-Item -Recurse -Force $tmpDir

# ── Summary ───────────────────────────────────────────────────────────────────

Write-Host ""
Write-Host "================================================================" -ForegroundColor Cyan
Write-Host "  Hardening restore complete" -ForegroundColor Cyan
Write-Host "  Backups: /etc/systemd/system/*.bak-$timestamp" -ForegroundColor Cyan
Write-Host "  Services: samverk-serve + samverk-dispatch (active)" -ForegroundColor Cyan
Write-Host "================================================================" -ForegroundColor Cyan
Write-Host ""
Write-Host "Next: close issue #226 once you've confirmed claude-cli auth works" -ForegroundColor Yellow
Write-Host "  ssh root@192.168.1.162 journalctl -u samverk-dispatch -f" -ForegroundColor DarkGray
