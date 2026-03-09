#Requires -Version 7.0
<#
.SYNOPSIS
    PC Agent autonomy gate — classifies issues into autonomy tiers and decides
    whether the PC agent may execute them autonomously.

.DESCRIPTION
    Implements ADR-015 (Three-Tier Autonomy Model) and ADR-021 (Intent Verification
    Protocol) for the PC agent runner. All filtering logic lives here.

    Tier Classification (based on labels):
        Tier 1 — complexity:local + any priority except critical
                 → Auto-execute, minimal logging
        Tier 2 — complexity:cloud + priority:normal or priority:high
                 → Auto-execute, log prominently
        Tier 3 — complexity:ambiguous OR priority:critical
                 → Skip, leave queued, post explanation comment

    Additional skip conditions (independent of tier):
        - agent:human label            → always skip
        - status:needs-human label     → already escalated, skip
        - depends_on references open issues  → skip (dependency unresolved)
        - pc-failure-N >= max_failures → skip (exhausted retries elsewhere)

    Default config (overridden by -Config hashtable):
        MaxTier      = 2     # Maximum tier the PC agent will auto-execute
        MaxFailures  = 2     # Maximum prior failures before skipping
        SkipLabels   = @('agent:human', 'status:needs-human')
#>

Set-StrictMode -Version 3.0
$ErrorActionPreference = 'Stop'

$script:ModuleDir = $PSScriptRoot

# Default autonomy configuration.
$script:DefaultAutonomyConfig = @{
    MaxTier     = 2
    MaxFailures = 2
    SkipLabels  = @('agent:human', 'status:needs-human')
}

# ---------------------------------------------------------------------------
# Frontmatter parsing
# ---------------------------------------------------------------------------

function Get-FrontmatterValue {
    <#
    .SYNOPSIS
        Extracts a scalar or list value from YAML frontmatter in an issue body.
    .DESCRIPTION
        Searches for `key: value` or `key: [v1, v2]` patterns in the YAML block
        between leading `---` delimiters. Returns $null if not found.
    .OUTPUTS
        string (scalar) or string[] (list value)
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)][string]$Body,
        [Parameter(Mandatory)][string]$Key
    )

    # Extract YAML block between opening --- and closing ---
    if ($Body -notmatch '(?s)^---\s*\r?\n(.+?)\r?\n---') {
        return $null
    }
    $yaml = $Matches[1]

    # Look for the key.
    if ($yaml -notmatch "(?m)^${Key}\s*:\s*(.+)$") {
        return $null
    }
    $raw = $Matches[1].Trim()

    # Inline list: [1, 2, 3]
    if ($raw -match '^\[(.+)\]$') {
        return $Matches[1] -split '\s*,\s*' | ForEach-Object { $_.Trim() }
    }

    return $raw
}

function Get-DependsOnNumbers {
    <#
    .SYNOPSIS
        Parses the depends_on list from an issue body's frontmatter.
    .OUTPUTS
        int[]  — may be empty.
    #>
    [CmdletBinding()]
    param([string]$Body)

    $raw = Get-FrontmatterValue -Body $Body -Key 'depends_on'
    if (-not $raw) { return @() }

    $nums = @()
    foreach ($item in @($raw)) {
        if ($item -match '^\d+$') {
            $nums += [int]$item
        }
    }
    return $nums
}

# ---------------------------------------------------------------------------
# Tier classification
# ---------------------------------------------------------------------------

function Get-IssueTier {
    <#
    .SYNOPSIS
        Classifies an issue into autonomy tier 1, 2, or 3 based on its labels.
    .DESCRIPTION
        Returns:
            1  — complexity:local (any non-critical priority)
            2  — complexity:cloud, priority:normal or priority:high
            3  — complexity:ambiguous, or priority:critical, or no complexity label
    .OUTPUTS
        int (1, 2, or 3)
    #>
    [CmdletBinding()]
    param([Parameter(Mandatory)][string[]]$Labels)

    $hasLocal     = 'complexity:local'     -in $Labels
    $hasCloud     = 'complexity:cloud'     -in $Labels
    $hasAmbiguous = 'complexity:ambiguous' -in $Labels
    $hasCritical  = 'priority:critical'    -in $Labels

    # Critical priority always escalates to Tier 3, regardless of complexity.
    if ($hasCritical) { return 3 }

    if ($hasLocal)     { return 1 }
    if ($hasCloud)     { return 2 }
    if ($hasAmbiguous) { return 3 }

    # No complexity label — treat as ambiguous.
    return 3
}

# ---------------------------------------------------------------------------
# Failure tracking (reads from labels, same as post-task.psm1)
# ---------------------------------------------------------------------------

function Get-PriorFailureCount {
    <#
    .SYNOPSIS
        Returns the pc-failure-N count stored in issue labels, or 0.
    #>
    [CmdletBinding()]
    param([string[]]$Labels)

    $label = $Labels | Where-Object { $_ -match '^pc-failure-(\d+)$' } | Select-Object -First 1
    if ($label -and $label -match '^pc-failure-(\d+)$') {
        return [int]$Matches[1]
    }
    return 0
}

# ---------------------------------------------------------------------------
# Main gate
# ---------------------------------------------------------------------------

function Test-AutonomyGate {
    <#
    .SYNOPSIS
        Decides whether the PC agent may execute an issue autonomously.
    .DESCRIPTION
        Evaluates all skip conditions in order:
            1. Hard skip labels (agent:human, status:needs-human, etc.)
            2. Tier classification — reject if tier > MaxTier
            3. Prior failure count — reject if >= MaxFailures
            4. Dependency check — reject if depends_on references open issues

        If -SkipDependencyCheck is set, step 4 is skipped (useful in tests
        and when the caller has already verified dependencies).
    .PARAMETER Issue
        PSCustomObject with Number, Title, Body, Labels. Returned by forge.psm1.
    .PARAMETER Config
        Autonomy configuration hashtable. Defaults to $script:DefaultAutonomyConfig.
    .PARAMETER ForgeConfig
        Forge configuration hashtable passed to Get-IssueDetails for dependency
        lookups. Required unless -SkipDependencyCheck is set.
    .PARAMETER SkipDependencyCheck
        Bypass the depends_on resolution step. Use in unit tests or when the
        caller guarantees dependencies are resolved.
    .OUTPUTS
        PSCustomObject:
            Allowed  bool    — $true if the agent may proceed
            Tier     int     — 1, 2, or 3 (0 if hard-skipped before tier check)
            Reason   string  — human-readable explanation (populated when Allowed=$false)
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)][PSCustomObject]$Issue,
        [hashtable]$Config = @{},
        [hashtable]$ForgeConfig = @{},
        [switch]$SkipDependencyCheck
    )

    $cfg = if ($Config.Count -eq 0) { $script:DefaultAutonomyConfig } else { $Config }

    $result = [PSCustomObject]@{
        Allowed = $false
        Tier    = 0
        Reason  = ''
    }

    # 1. Hard skip labels.
    $skipLabels = @($cfg.SkipLabels)
    foreach ($skipLabel in $skipLabels) {
        if ($skipLabel -in $Issue.Labels) {
            $result.Reason = "Issue has skip label '$skipLabel'."
            return $result
        }
    }

    # 2. Tier classification.
    $tier = Get-IssueTier -Labels $Issue.Labels
    $result.Tier = $tier

    if ($tier -gt $cfg.MaxTier) {
        $result.Reason = "Tier $tier exceeds max allowed tier $($cfg.MaxTier)."
        return $result
    }

    # 3. Prior failure count.
    $failures = Get-PriorFailureCount -Labels $Issue.Labels
    if ($failures -ge $cfg.MaxFailures) {
        $result.Reason = "Issue has $failures prior failure(s) (max: $($cfg.MaxFailures))."
        return $result
    }

    # 4. Dependency check.
    if (-not $SkipDependencyCheck) {
        $depNums = Get-DependsOnNumbers -Body $Issue.Body
        if ($depNums.Count -gt 0) {
            # Import forge lazily (not available in all test contexts).
            $forgeModule = Join-Path $script:ModuleDir 'forge.psm1'
            if (Test-Path $forgeModule) {
                Import-Module $forgeModule -Force -ErrorAction SilentlyContinue
            }

            $resolvedCfg = if ($ForgeConfig.Count -gt 0) { $ForgeConfig } else { Get-ForgeConfig }

            $unresolvedDeps = @()
            foreach ($depNum in $depNums) {
                try {
                    $dep = Get-IssueDetails -IssueNumber $depNum -Config $resolvedCfg
                    if ($dep -and $dep.PSObject.Properties['State'] -and $dep.State -eq 'open') {
                        $unresolvedDeps += $depNum
                    } elseif ($dep -and -not $dep.PSObject.Properties['State']) {
                        # No State field — treat as open (conservative).
                        $unresolvedDeps += $depNum
                    }
                } catch {
                    Write-Verbose "Could not resolve dependency #${depNum}: ${_}"
                    $unresolvedDeps += $depNum
                }
            }

            if ($unresolvedDeps.Count -gt 0) {
                $depList = $unresolvedDeps -join ', '
                $result.Reason = "Unresolved dependencies: #$depList"
                return $result
            }
        }
    }

    # All checks passed.
    $result.Allowed = $true
    return $result
}

# ---------------------------------------------------------------------------
# Exports
# ---------------------------------------------------------------------------

Export-ModuleMember -Function @(
    'Get-IssueTier',
    'Get-DependsOnNumbers',
    'Get-FrontmatterValue',
    'Get-PriorFailureCount',
    'Test-AutonomyGate'
)
