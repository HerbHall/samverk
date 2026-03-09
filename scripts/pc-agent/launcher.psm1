#Requires -Version 7.0
<#
.SYNOPSIS
    PC Agent CC launcher — provisions a worktree, formats the prompt, runs Claude
    Code headlessly, and returns a structured result.

.DESCRIPTION
    Combines workspace management, prompt formatting, and CC invocation into a
    single pipeline. The worktree is NOT cleaned up by this module — the caller
    (post-task handler) is responsible for cleanup after inspecting the result.

    CC invocation:
        claude --print --dangerously-skip-permissions --output-format json --max-turns 50

    Rationale (from #304 research): --print enables headless agentic mode;
    --dangerously-skip-permissions removes interactive prompts; --output-format json
    produces parseable structured output; --max-turns 50 prevents infinite loops.
#>

Set-StrictMode -Version 3.0
$ErrorActionPreference = 'Stop'

# Import dependencies.
$script:ModuleDir = $PSScriptRoot
if (-not (Get-Module workspace)) {
    Import-Module (Join-Path $script:ModuleDir 'workspace.psm1') -Global
}
Import-Module (Join-Path $script:ModuleDir 'formatter.psm1') -Force

# Default timeouts by complexity label.
$script:TimeoutsByComplexity = @{
    'complexity:local'      = 600    # 10 minutes
    'complexity:cloud'      = 1800   # 30 minutes
    'complexity:ambiguous'  = 900    # 15 minutes (default)
}
$script:DefaultTimeoutSeconds = 900

function Get-TaskTimeout {
    <#
    .SYNOPSIS
        Returns the appropriate timeout in seconds based on issue labels.
    #>
    [CmdletBinding()]
    param([string[]]$Labels)

    foreach ($label in $Labels) {
        if ($script:TimeoutsByComplexity.ContainsKey($label)) {
            return $script:TimeoutsByComplexity[$label]
        }
    }
    return $script:DefaultTimeoutSeconds
}

function Invoke-CCTask {
    <#
    .SYNOPSIS
        Runs Claude Code on a Samverk issue in an isolated worktree.
    .DESCRIPTION
        Provisions a new worktree, formats the issue as a CC prompt, launches CC
        with a timeout, and returns a structured result. The worktree is preserved
        after this call — the post-task handler decides whether to keep or remove it.
    .PARAMETER Issue
        PSCustomObject with Number, Title, Body, Labels, AgentType.
    .PARAMETER WorkspaceConfig
        Workspace configuration hashtable (passed to workspace.psm1 functions).
    .PARAMETER MaxTurns
        Maximum number of CC agentic turns. Default: 50.
    .OUTPUTS
        PSCustomObject with:
            Success       bool
            Output        string (raw CC output)
            ParsedOutput  hashtable (from --output-format json; may be null)
            ExitCode      int
            Duration      TimeSpan
            Branch        string
            WorktreePath  string
            Slot          string
            Error         string (error message if failed)
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)][PSCustomObject]$Issue,
        [hashtable]$WorkspaceConfig = @{},
        [int]$MaxTurns = 50
    )

    # Resolve workspace config.
    $wsConfig = if ($WorkspaceConfig.Count -eq 0) { Get-WorkspaceConfig } else { $WorkspaceConfig }

    # Fetch latest code before creating worktree.
    Update-BareRepo -Config $wsConfig

    # Provision worktree.
    $worktree = New-AgentWorktree `
        -IssueNumber $Issue.Number `
        -IssueTitle  $Issue.Title  `
        -Config      $wsConfig

    $startTime = Get-Date
    $result = [PSCustomObject]@{
        Success      = $false
        Output       = ''
        ParsedOutput = $null
        ExitCode     = -1
        Duration     = [TimeSpan]::Zero
        Branch       = $worktree.Branch
        WorktreePath = $worktree.Path
        Slot         = $worktree.Slot
        Error        = ''
    }

    try {
        # Format the task prompt.
        $prompt = Format-IssuePrompt -Issue $Issue -Branch $worktree.Branch

        # Determine timeout.
        $timeoutSec = Get-TaskTimeout -Labels $Issue.Labels

        Write-Host "Launching CC for issue #$($Issue.Number) in $($worktree.Path) (timeout: ${timeoutSec}s, max-turns: $MaxTurns)"

        # Launch CC as a background job so we can enforce the timeout.
        $worktreePath = $worktree.Path
        $job = Start-Job -ScriptBlock {
            param($Path, $Prompt, $MaxTurns)
            Set-Location $Path
            # Unset CLAUDECODE so CC can launch from inside another CC session.
            Remove-Item Env:CLAUDECODE -ErrorAction SilentlyContinue
            $out = & claude `
                --print `
                --dangerously-skip-permissions `
                --output-format json `
                --max-turns $MaxTurns `
                $Prompt 2>&1 | Out-String
            return [PSCustomObject]@{ Output = $out; ExitCode = $LASTEXITCODE }
        } -ArgumentList $worktreePath, $prompt, $MaxTurns

        $completed = Wait-Job $job -Timeout $timeoutSec
        $result.Duration = (Get-Date) - $startTime

        if (-not $completed) {
            # Timeout — kill the job and the CC process.
            Stop-Job $job
            Remove-Job $job -Force
            $result.Error = "CC task timed out after ${timeoutSec}s"
            Write-Warning $result.Error
            return $result
        }

        $jobResult = Receive-Job $job
        Remove-Job $job -Force

        $result.Output   = $jobResult.Output
        $result.ExitCode = $jobResult.ExitCode

        # Parse JSON output.
        $parsed = $result.Output.Trim() | ConvertFrom-Json -ErrorAction SilentlyContinue
        $result.ParsedOutput = $parsed

        if ($result.ExitCode -eq 0 -and (-not $parsed -or -not $parsed.is_error)) {
            $result.Success = $true
        } else {
            $errorMsg = if ($parsed) { $parsed.result } else { $result.Output }
            $result.Error = "CC exited with code $($result.ExitCode): $errorMsg"
        }

    } catch {
        $result.Duration = (Get-Date) - $startTime
        $result.Error    = "Launcher exception: $($_.Exception.Message)"
        Write-Warning $result.Error
    }

    return $result
}

Export-ModuleMember -Function @('Invoke-CCTask', 'Get-TaskTimeout')
