#Requires -Version 7.0
<#
.SYNOPSIS
    PC Agent post-task handler — pushes changes, opens a PR, updates the issue,
    and cleans up the worktree after a CC task completes.

.DESCRIPTION
    Handles all four outcome states after CC completes in the worktree:
        - Success with commits pushed by CC
        - Success with commits not yet pushed (CC committed but didn't push)
        - Success with staged-but-uncommitted changes (auto-commit + push)
        - No changes (CC found nothing to do)
        - Failure (CC exited with error or timeout)

    Label lifecycle:
        status:active  → status:needs-qc    (success → ready for QC review)
        status:active  → status:queued      (first failure → re-queue)
        status:active  → status:needs-human (max failures reached)
#>

Set-StrictMode -Version 3.0
$ErrorActionPreference = 'Stop'

$script:ModuleDir = Split-Path $PSCommandPath -Parent
Import-Module (Join-Path $script:ModuleDir 'workspace.psm1') -Force
Import-Module (Join-Path $script:ModuleDir 'forge.psm1') -Force

# Maximum number of consecutive failures before escalating to human.
$script:MaxFailures = 2

# ---------------------------------------------------------------------------
# Git state detection
# ---------------------------------------------------------------------------

function Get-WorktreeGitState {
    <#
    .SYNOPSIS
        Inspects the worktree and returns its current git state.
    .OUTPUTS
        PSCustomObject with:
            HasCommits    bool   — branch has commits not on main
            HasPushed     bool   — branch exists on remote
            HasStaged     bool   — staged but uncommitted changes exist
            HasUntracked  bool   — untracked files present
            CommitCount   int    — commits ahead of main
            LastMessage   string — last commit message (or empty)
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)][string]$WorktreePath,
        [Parameter(Mandatory)][string]$Branch
    )

    # Commits ahead of main.
    $aheadRaw = & git -C $WorktreePath rev-list --count "origin/main..HEAD" 2>&1 | Out-String
    $commitCount = if ($aheadRaw -match '^\d+') { [int]$aheadRaw.Trim() } else { 0 }

    # Check if branch exists on remote.
    $remoteCheck = & git -C $WorktreePath ls-remote --heads origin $Branch 2>&1 | Out-String
    $hasPushed = $remoteCheck.Trim().Length -gt 0

    # Staged changes.
    $stagedRaw = & git -C $WorktreePath diff --cached --name-only 2>&1 | Out-String
    $hasStaged = $stagedRaw.Trim().Length -gt 0

    # Untracked files.
    $untrackedRaw = & git -C $WorktreePath ls-files --others --exclude-standard 2>&1 | Out-String
    $hasUntracked = $untrackedRaw.Trim().Length -gt 0

    # Last commit message.
    $lastMsg = ''
    if ($commitCount -gt 0) {
        $lastMsg = (& git -C $WorktreePath log -1 --pretty=%s 2>&1 | Out-String).Trim()
    }

    return [PSCustomObject]@{
        HasCommits   = $commitCount -gt 0
        HasPushed    = $hasPushed
        HasStaged    = $hasStaged
        HasUntracked = $hasUntracked
        CommitCount  = $commitCount
        LastMessage  = $lastMsg
    }
}

# ---------------------------------------------------------------------------
# Branch cleanup
# ---------------------------------------------------------------------------

function Remove-AgentBranch {
    <#
    .SYNOPSIS
        Deletes the local branch from the bare repo after worktree removal.
    .PARAMETER Branch
        Branch name to delete (e.g. fix/42-add-feature).
    .PARAMETER BareRepo
        Path to the bare repo.
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)][string]$Branch,
        [Parameter(Mandatory)][string]$BareRepo
    )

    # Only delete the local branch — leave the remote branch for PR review.
    $null = & git -C $BareRepo branch -D $Branch 2>&1
    Write-Verbose "Deleted local branch $Branch from bare repo."
}

# ---------------------------------------------------------------------------
# Failure tracking
# ---------------------------------------------------------------------------

function Get-FailureCount {
    <#
    .SYNOPSIS
        Reads the current failure count from issue labels.
    .DESCRIPTION
        Failure count is stored as a label `pc-failure-N` on the issue.
        Returns 0 if no such label exists.
    .PARAMETER Labels
        Array of current label names on the issue.
    #>
    [CmdletBinding()]
    param([string[]]$Labels)

    $failureLabel = $Labels | Where-Object { $_ -match '^pc-failure-(\d+)$' } | Select-Object -First 1
    if ($failureLabel -and $failureLabel -match '^pc-failure-(\d+)$') {
        return [int]$Matches[1]
    }
    return 0
}

function Set-FailureCount {
    <#
    .SYNOPSIS
        Updates the pc-failure-N label on an issue.
    .PARAMETER IssueNumber
        Issue to update.
    .PARAMETER Count
        New failure count.
    .PARAMETER CurrentLabels
        Current labels (used to find and remove old failure label).
    .PARAMETER Config
        Forge config.
    #>
    [CmdletBinding()]
    param(
        [int]$IssueNumber,
        [int]$Count,
        [string[]]$CurrentLabels,
        [hashtable]$Config
    )

    $removeLabels = @($CurrentLabels | Where-Object { $_ -match '^pc-failure-\d+$' })
    $addLabels    = @("pc-failure-$Count")
    Update-IssueStatus -IssueNumber $IssueNumber `
        -AddLabels $addLabels -RemoveLabels $removeLabels -Config $Config
}

# ---------------------------------------------------------------------------
# Main handler
# ---------------------------------------------------------------------------

function Invoke-PostTask {
    <#
    .SYNOPSIS
        Processes CC task results: push, open PR, update issue, cleanup.
    .DESCRIPTION
        Handles the full post-task lifecycle based on the CCResult and worktree
        git state. Always cleans up the worktree at the end (unless
        PreserveOnFailure is set and the task failed).
    .PARAMETER Issue
        PSCustomObject with Number, Title, Labels, AgentType.
    .PARAMETER CCResult
        PSCustomObject returned by Invoke-CCTask (from launcher.psm1).
    .PARAMETER ForgeConfig
        Forge configuration hashtable.
    .PARAMETER WorkspaceConfig
        Workspace configuration hashtable.
    .PARAMETER PreserveOnFailure
        When $true and the task failed, skip worktree removal (for debugging).
    .OUTPUTS
        PSCustomObject with Success (bool), PRNumber (int), Comment (string).
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)][PSCustomObject]$Issue,
        [Parameter(Mandatory)][PSCustomObject]$CCResult,
        [hashtable]$ForgeConfig    = @{},
        [hashtable]$WorkspaceConfig = @{},
        [switch]$PreserveOnFailure
    )

    $fConfig = if ($ForgeConfig.Count -eq 0) { Get-ForgeConfig } else { $ForgeConfig }
    $wConfig = if ($WorkspaceConfig.Count -eq 0) { Get-WorkspaceConfig } else { $WorkspaceConfig }

    $result = [PSCustomObject]@{
        Success  = $false
        PRNumber = 0
        Comment  = ''
    }

    # --- Failure path ---
    if (-not $CCResult.Success) {
        Write-Host "Task failed for issue #$($Issue.Number): $($CCResult.Error)"

        $currentIssue = Get-IssueDetails -IssueNumber $Issue.Number -Config $fConfig
        $failCount    = (Get-FailureCount -Labels $currentIssue.Labels) + 1
        Set-FailureCount -IssueNumber $Issue.Number -Count $failCount `
            -CurrentLabels $currentIssue.Labels -Config $fConfig

        $durationStr = '{0:N0}m {1:N0}s' -f [Math]::Floor($CCResult.Duration.TotalMinutes), ($CCResult.Duration.Seconds % 60)
        $errorExcerpt = if ($CCResult.Error.Length -gt 500) { $CCResult.Error.Substring(0, 500) + '...' } else { $CCResult.Error }

        if ($failCount -ge $script:MaxFailures) {
            # Escalate to human.
            $comment = "**[PC Agent] Task failed — escalated after $failCount attempt(s)**`n`nError: $errorExcerpt`n`nDuration: $durationStr`nWorker: $($CCResult.Slot)"
            Add-IssueComment -IssueNumber $Issue.Number -Body $comment -Config $fConfig
            Update-IssueStatus -IssueNumber $Issue.Number `
                -AddLabels @('status:needs-human') `
                -RemoveLabels @('status:active') `
                -Config $fConfig
        } else {
            # Re-queue for another attempt.
            $comment = "**[PC Agent] Task failed — re-queuing (attempt $failCount of $($script:MaxFailures))**`n`nError: $errorExcerpt`n`nDuration: $durationStr`nWorker: $($CCResult.Slot)"
            Add-IssueComment -IssueNumber $Issue.Number -Body $comment -Config $fConfig
            Update-IssueStatus -IssueNumber $Issue.Number `
                -AddLabels @('status:queued') `
                -RemoveLabels @('status:active') `
                -Config $fConfig
        }

        $result.Comment = $comment

        # Cleanup (unless preserving for debug).
        if (-not $PreserveOnFailure -or $CCResult.Slot -eq '') {
            Remove-AgentWorktree -Slot $CCResult.Slot -Config $wConfig
        } else {
            Write-Host "PreserveOnFailure: keeping worktree at $($CCResult.WorktreePath) for debugging."
        }
        return $result
    }

    # --- Success path: inspect worktree git state ---
    $gitState = Get-WorktreeGitState -WorktreePath $CCResult.WorktreePath -Branch $CCResult.Branch

    # Handle AGENT_BLOCKED.md
    $blockedFile = Join-Path $CCResult.WorktreePath 'AGENT_BLOCKED.md'
    if (Test-Path $blockedFile) {
        $blockedContent = Get-Content $blockedFile -Raw -ErrorAction SilentlyContinue
        $comment = "**[PC Agent] Blocked**`n`n$blockedContent`n`nWorker: $($CCResult.Slot)"
        Add-IssueComment -IssueNumber $Issue.Number -Body $comment -Config $fConfig
        Update-IssueStatus -IssueNumber $Issue.Number `
            -AddLabels @('status:needs-human') `
            -RemoveLabels @('status:active') `
            -Config $fConfig
        $result.Comment = $comment
        Remove-AgentWorktree -Slot $CCResult.Slot -Config $wConfig
        return $result
    }

    # No changes at all.
    if (-not $gitState.HasCommits -and -not $gitState.HasStaged) {
        $comment = "**[PC Agent] No changes made**`n`nCC completed but made no file changes. This may indicate the issue was already resolved or the task needs clarification.`n`nDuration: $('{0:N0}m {1:N0}s' -f [Math]::Floor($CCResult.Duration.TotalMinutes), ($CCResult.Duration.Seconds % 60))`nWorker: $($CCResult.Slot)"
        Add-IssueComment -IssueNumber $Issue.Number -Body $comment -Config $fConfig
        Update-IssueStatus -IssueNumber $Issue.Number `
            -AddLabels @('status:needs-human') `
            -RemoveLabels @('status:active') `
            -Config $fConfig
        $result.Comment = $comment
        Remove-AgentWorktree -Slot $CCResult.Slot -Config $wConfig
        return $result
    }

    # Staged but not committed — auto-commit.
    if ($gitState.HasStaged -and -not $gitState.HasCommits) {
        Write-Verbose "Auto-committing staged changes in worktree..."
        $null = & git -C $CCResult.WorktreePath commit -m "feat(#$($Issue.Number)): agent changes (auto-committed by post-task handler)" 2>&1
        if ($LASTEXITCODE -ne 0) {
            Write-Warning "Auto-commit failed — treating as no-changes."
        }
        # Refresh state after commit.
        $gitState = Get-WorktreeGitState -WorktreePath $CCResult.WorktreePath -Branch $CCResult.Branch
    }

    # Push branch if CC didn't push.
    if ($gitState.HasCommits -and -not $gitState.HasPushed) {
        Write-Host "Pushing branch $($CCResult.Branch) ..."
        $null = & git -C $CCResult.WorktreePath push origin $CCResult.Branch 2>&1
        if ($LASTEXITCODE -ne 0) {
            Write-Warning "Push failed — will retry once after 10 seconds."
            Start-Sleep -Seconds 10
            $null = & git -C $CCResult.WorktreePath push origin $CCResult.Branch 2>&1
            if ($LASTEXITCODE -ne 0) {
                $comment = "**[PC Agent] Push failed**`n`nBranch `$($CCResult.Branch)` has commits but could not be pushed. Manual push required.`n`nWorker: $($CCResult.Slot)"
                Add-IssueComment -IssueNumber $Issue.Number -Body $comment -Config $fConfig
                Update-IssueStatus -IssueNumber $Issue.Number `
                    -AddLabels @('status:needs-human') `
                    -RemoveLabels @('status:active') `
                    -Config $fConfig
                $result.Comment = $comment
                Remove-AgentWorktree -Slot $CCResult.Slot -Config $wConfig
                return $result
            }
        }
    }

    # Open PR.
    $durationStr = '{0:N0}m {1:N0}s' -f [Math]::Floor($CCResult.Duration.TotalMinutes), ($CCResult.Duration.Seconds % 60)
    $prTitle = if ($gitState.LastMessage -ne '') { $gitState.LastMessage } else { "fix(#$($Issue.Number)): $($Issue.Title)" }

    try {
        $prNumber = Open-PullRequest -IssueNumber $Issue.Number -Branch $CCResult.Branch -Title $prTitle -Config $fConfig
    } catch {
        $prNumber = 0
        Write-Warning "PR creation failed: $($_.Exception.Message)"
    }

    # Update issue labels.
    Update-IssueStatus -IssueNumber $Issue.Number `
        -AddLabels @('status:needs-qc') `
        -RemoveLabels @('status:active') `
        -Config $fConfig

    # Post completion comment.
    $prRef = if ($prNumber -gt 0) { "PR #$prNumber opened." } else { "PR creation failed — branch $($CCResult.Branch) pushed, create PR manually." }
    $comment = "**[PC Agent] Task complete**`n`n$prRef`n`nDuration: $durationStr`nCommits: $($gitState.CommitCount)`nWorker: $($CCResult.Slot)"
    Add-IssueComment -IssueNumber $Issue.Number -Body $comment -Config $fConfig

    $result.Success  = $true
    $result.PRNumber = $prNumber
    $result.Comment  = $comment

    # Cleanup worktree and local branch.
    Remove-AgentWorktree -Slot $CCResult.Slot -Config $wConfig
    if (Test-Path (Get-BareRepoPath -Config $wConfig)) {
        Remove-AgentBranch -Branch $CCResult.Branch -BareRepo (Get-BareRepoPath -Config $wConfig)
    }

    Write-Host "Issue #$($Issue.Number) complete. $($result.Comment)"
    return $result
}

Export-ModuleMember -Function @('Invoke-PostTask', 'Get-WorktreeGitState')
