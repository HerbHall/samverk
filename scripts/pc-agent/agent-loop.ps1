#Requires -Version 7.0
<#
.SYNOPSIS
    PC Agent runner loop -- polls for claimable issues, provisions worktrees,
    runs Claude Code, and cleans up. Ties together all pc-agent modules.

.DESCRIPTION
    Orchestrates the full PC agent lifecycle for a single worker slot:

        poll -> autonomy gate -> claim -> Invoke-CCTask -> Invoke-PostTask -> sleep -> repeat

    Modes:
        --SingleRun   Process at most one issue, then exit. Useful for testing
                      and for scheduled execution (e.g., Windows Task Scheduler).
        --Continuous  Run indefinitely until stopped (Ctrl-C or SIGTERM).
                      This is the production mode.

    The script does NOT manage concurrency. Run multiple instances with
    different -Slot values for parallel processing (Phase 6, issue #315).

    Exit codes:
        0   Clean exit (SingleRun with no work, or graceful Ctrl-C)
        1   Initialization failure (workspace not set up)
        2   SingleRun processed one issue (regardless of task success)

.PARAMETER SingleRun
    Exit after the first poll cycle (with or without work).

.PARAMETER Continuous
    Run indefinitely. Default when neither switch is supplied.

.PARAMETER PollIntervalSeconds
    Seconds to sleep between polls when no work is found. Default: 60.

.PARAMETER PreserveOnFailure
    Pass-through to Invoke-PostTask: keeps the worktree after a task failure
    for debugging. The slot will be unusable until the worktree is manually
    removed. Do NOT use in production.

.PARAMETER InitWorkspace
    Run Initialize-AgentWorkspace before the loop (one-time setup).
    Safe to pass on every invocation -- the function is idempotent.

.EXAMPLE
    # One-off test: process a single issue and exit.
    .\agent-loop.ps1 -SingleRun

.EXAMPLE
    # Production: poll every 90 seconds indefinitely.
    .\agent-loop.ps1 -Continuous -PollIntervalSeconds 90

.EXAMPLE
    # First-time setup + single run.
    .\agent-loop.ps1 -InitWorkspace -SingleRun
#>

[CmdletBinding(DefaultParameterSetName = 'Continuous')]
param(
    [Parameter(ParameterSetName = 'Single')][switch]$SingleRun,
    [Parameter(ParameterSetName = 'Continuous')][switch]$Continuous,
    [int]$PollIntervalSeconds = 60,
    [switch]$PreserveOnFailure,
    [switch]$InitWorkspace
)

Set-StrictMode -Version 3.0
$ErrorActionPreference = 'Stop'

# ---------------------------------------------------------------------------
# Module imports
# ---------------------------------------------------------------------------

$script:ModuleDir = Split-Path $PSCommandPath -Parent

Import-Module (Join-Path $script:ModuleDir 'workspace.psm1')  -Force
Import-Module (Join-Path $script:ModuleDir 'forge.psm1')      -Force
Import-Module (Join-Path $script:ModuleDir 'launcher.psm1')   -Force
Import-Module (Join-Path $script:ModuleDir 'post-task.psm1')  -Force
Import-Module (Join-Path $script:ModuleDir 'autonomy.psm1')   -Force

# ---------------------------------------------------------------------------
# Graceful shutdown
# ---------------------------------------------------------------------------

$script:StopRequested = $false

# Trap Ctrl-C so the loop can finish the current task before exiting.
$null = [Console]::TreatControlCAsInput = $false
Register-EngineEvent -SourceIdentifier 'PowerShell.Exiting' -Action {
    $script:StopRequested = $true
} | Out-Null

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

function Write-AgentLog {
    param([string]$Message, [string]$Level = 'INFO')
    $ts = (Get-Date).ToString('yyyy-MM-dd HH:mm:ss')
    Write-Host "[$ts] [$Level] [agent-loop] $Message"
}

function Invoke-PollCycle {
    <#
    .SYNOPSIS
        One full poll-claim-execute cycle. Returns $true if an issue was processed,
        $false if the queue was empty or all issues were gated.
    #>
    [CmdletBinding()]
    param(
        [hashtable]$ForgeConfig,
        [hashtable]$WorkspaceConfig,
        [hashtable]$AutonomyConfig = @{},
        [switch]$PreserveOnFailure
    )

    # 1. Check for available worker slot.
    $slot = Get-AvailableWorkerSlot -Config $WorkspaceConfig
    if (-not $slot) {
        Write-AgentLog 'All worker slots occupied -- skipping poll.' 'WARN'
        return $false
    }

    # 2. Poll for claimable issues.
    Write-AgentLog 'Polling for claimable issues...'
    $issues = Get-ClaimableIssues -Config $ForgeConfig
    if (-not $issues -or $issues.Count -eq 0) {
        Write-AgentLog 'No claimable issues found.'
        return $false
    }

    # 3. Evaluate each issue through the autonomy gate; pick the first that passes.
    $issue = $null
    foreach ($candidate in $issues) {
        Write-AgentLog "Evaluating issue #$($candidate.Number): $($candidate.Title)"
        $gate = Test-AutonomyGate -Issue $candidate -Config $AutonomyConfig `
            -ForgeConfig $ForgeConfig -SkipDependencyCheck
        if ($gate.Allowed) {
            $issue = $candidate
            Write-AgentLog "Autonomy gate passed (Tier $($gate.Tier)) for issue #$($candidate.Number)."
            break
        } else {
            Write-AgentLog "Skipping issue #$($candidate.Number): $($gate.Reason)" 'WARN'
            # For Tier 3 issues, post an informational comment so the author knows why.
            if ($gate.Tier -ge 3) {
                $gateComment = "**[PC Agent] Autonomy gate: skipping**`n`n" +
                    "This issue requires human review ($($gate.Reason)). Leaving queued."
                try {
                    Add-IssueComment -IssueNumber $candidate.Number -Body $gateComment -Config $ForgeConfig
                } catch {
                    Write-AgentLog "Could not post gate comment on #$($candidate.Number): $_" 'WARN'
                }
            }
        }
    }

    if (-not $issue) {
        Write-AgentLog 'No issues passed the autonomy gate in this poll cycle.'
        return $false
    }

    # 4. Claim the issue (race-safe -- another agent may beat us).
    $claimed = Claim-Issue -IssueNumber $issue.Number -Config $ForgeConfig
    if (-not $claimed) {
        Write-AgentLog "Issue #$($issue.Number) was claimed by another agent -- skipping." 'WARN'
        return $false
    }
    Write-AgentLog "Claimed issue #$($issue.Number)."

    # 5. Run CC in an isolated worktree.
    Write-AgentLog "Launching CC task for issue #$($issue.Number)..."
    $ccResult = Invoke-CCTask -Issue $issue -WorkspaceConfig $WorkspaceConfig

    $durationStr = '{0:N0}m {1:N0}s' -f `
        [Math]::Floor($ccResult.Duration.TotalMinutes), ($ccResult.Duration.Seconds % 60)
    Write-AgentLog "CC task finished in $durationStr. Success=$($ccResult.Success)"

    # 6. Post-task: push, PR, label updates, cleanup.
    $ptParams = @{
        Issue           = $issue
        CCResult        = $ccResult
        ForgeConfig     = $ForgeConfig
        WorkspaceConfig = $WorkspaceConfig
    }
    if ($PreserveOnFailure) { $ptParams['PreserveOnFailure'] = $true }

    $postResult = Invoke-PostTask @ptParams
    Write-AgentLog "Post-task complete. Success=$($postResult.Success) PR=#$($postResult.PRNumber)"

    return $true
}

# ---------------------------------------------------------------------------
# Entry point
# ---------------------------------------------------------------------------

Write-AgentLog "PC Agent starting. Mode=$(if ($SingleRun) { 'SingleRun' } else { 'Continuous' }) PollInterval=${PollIntervalSeconds}s"

# One-time workspace initialisation (idempotent).
if ($InitWorkspace) {
    Write-AgentLog 'Initializing agent workspace...'
    $null = Initialize-AgentWorkspace
    Write-AgentLog 'Workspace ready.'
}

# Validate workspace exists.
$wsConfig     = Get-WorkspaceConfig
$bareRepoPath = Join-Path $wsConfig.Root $wsConfig.BareRepo
if (-not (Test-Path $bareRepoPath)) {
    Write-AgentLog "Bare repo not found at '$bareRepoPath'. Run with -InitWorkspace first." 'ERROR'
    exit 1
}

$autonomyConfig = @{}
$forgeConfig    = Get-ForgeConfig

# ---------------------------------------------------------------------------
# Main loop
# ---------------------------------------------------------------------------

$iterationCount = 0

while (-not $script:StopRequested) {
    $iterationCount++
    Write-AgentLog "--- Poll cycle #$iterationCount ---"

    try {
        $didWork = Invoke-PollCycle `
            -ForgeConfig     $forgeConfig `
            -WorkspaceConfig $wsConfig `
            -AutonomyConfig  $autonomyConfig `
            -PreserveOnFailure:$PreserveOnFailure
    } catch {
        Write-AgentLog "Unhandled error in poll cycle: $($_.Exception.Message)" 'ERROR'
        Write-AgentLog $_.ScriptStackTrace 'ERROR'
        $didWork = $false
    }

    # SingleRun: exit after first cycle regardless of whether work was done.
    if ($SingleRun) {
        $exitCode = if ($didWork) { 2 } else { 0 }
        Write-AgentLog "SingleRun complete. ExitCode=$exitCode"
        exit $exitCode
    }

    # Continuous: sleep between polls. Skip sleep if we just did work
    # (there may be more issues waiting).
    if (-not $didWork) {
        Write-AgentLog "Sleeping ${PollIntervalSeconds}s before next poll..."
        Start-Sleep -Seconds $PollIntervalSeconds
    }
}

Write-AgentLog 'Stop requested -- exiting cleanly.'
exit 0
