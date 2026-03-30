#Requires -Version 7.0
<#
.SYNOPSIS
    PC Agent forge poller — queries GitHub/Gitea for claimable issues and manages
    issue lifecycle labels.

.DESCRIPTION
    Supports both GitHub (via gh CLI) and Gitea (via Invoke-RestMethod with PAT auth).
    The active forge is selected by the `forge` key in config.

    Label lifecycle:
        status:queued  → status:claimed     (claimed by this agent)
        status:claimed → status:needs-qc    (task completed successfully)
        status:claimed → status:needs-human (task failed or blocked)
#>

Set-StrictMode -Version 3.0
$ErrorActionPreference = 'Stop'

# ---------------------------------------------------------------------------
# Configuration
# ---------------------------------------------------------------------------

$script:DefaultForgeConfig = @{
    Forge           = 'github'           # 'github' or 'gitea'
    GiteaUrl        = 'http://gitea.herbhall.net'
    GiteaTokenEnv   = 'GITEA_TOKEN'
    Project         = 'HerbHall/samverk'
    AgentId         = 'pc-worker'
    PollInterval    = 60
    AgentLabels     = @('agent:code-gen', 'agent:triage', 'agent:research', 'agent:docs', 'agent:pc')
}

function Get-ForgeConfig {
    <#
    .SYNOPSIS
        Returns forge configuration, merging defaults with environment overrides.
    .DESCRIPTION
        Environment variables override hardcoded defaults so PC workers can be
        configured per-deployment (Docker, different forges, multi-project).
        Env vars: SAMVERK_FORGE, SAMVERK_FORGE_URL, SAMVERK_FORGE_PROJECT,
        SAMVERK_AGENT_ID.
    #>
    [CmdletBinding()]
    param([string]$YamlPath = '')

    $config = $script:DefaultForgeConfig.Clone()

    if ($env:SAMVERK_FORGE)         { $config.Forge    = $env:SAMVERK_FORGE }
    if ($env:SAMVERK_FORGE_URL)     { $config.GiteaUrl = $env:SAMVERK_FORGE_URL }
    if ($env:SAMVERK_FORGE_PROJECT) { $config.Project  = $env:SAMVERK_FORGE_PROJECT }
    if ($env:SAMVERK_AGENT_ID)      { $config.AgentId  = $env:SAMVERK_AGENT_ID }

    return $config
}

# ---------------------------------------------------------------------------
# Internal helpers
# ---------------------------------------------------------------------------

function Invoke-GiteaAPI {
    <#
    .SYNOPSIS
        Calls the Gitea REST API with PAT authentication.
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)][string]$Path,
        [string]$Method = 'GET',
        [hashtable]$Body = @{},
        [hashtable]$Config = $script:DefaultForgeConfig
    )

    $token = [Environment]::GetEnvironmentVariable($Config.GiteaTokenEnv)
    if (-not $token) {
        throw "Gitea token not set. Set the '$($Config.GiteaTokenEnv)' environment variable."
    }

    $headers = @{ Authorization = "token $token"; 'Content-Type' = 'application/json' }
    $uri     = "$($Config.GiteaUrl)/api/v1$Path"

    $params = @{ Uri = $uri; Method = $Method; Headers = $headers; TimeoutSec = 30 }
    if ($Method -in @('POST', 'PATCH', 'PUT') -and $Body.Count -gt 0) {
        $params.Body = ($Body | ConvertTo-Json -Depth 5)
    }

    try {
        return Invoke-RestMethod @params
    } catch [System.Net.Http.HttpRequestException] {
        throw "Gitea API request failed: $($_.Exception.Message)"
    }
}

function Invoke-GitHubCLI {
    <#
    .SYNOPSIS
        Calls gh CLI and returns parsed JSON output.
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)][string[]]$Args
    )

    $tmpErr = [System.IO.Path]::GetTempFileName()
    try {
        $out = & gh @Args 2>$tmpErr | Out-String
        if ($LASTEXITCODE -ne 0) {
            $err = Get-Content $tmpErr -Raw -ErrorAction SilentlyContinue
            throw "gh CLI failed (exit $LASTEXITCODE): $err"
        }
        return $out.Trim()
    } finally {
        Remove-Item $tmpErr -ErrorAction SilentlyContinue
    }
}

# ---------------------------------------------------------------------------
# Public API
# ---------------------------------------------------------------------------

function Get-ClaimableIssues {
    <#
    .SYNOPSIS
        Returns issues with status:queued that are claimable by this agent.
    .DESCRIPTION
        Filters by status:queued label. Only returns issues whose agent label
        matches one of the configured AgentLabels. Returns as PSCustomObject list.
    .PARAMETER Config
        Forge configuration hashtable.
    .OUTPUTS
        Array of PSCustomObject with Number, Title, Body, Labels, AgentType
    #>
    [CmdletBinding()]
    param([hashtable]$Config = $script:DefaultForgeConfig)

    $owner, $repo = $Config.Project -split '/', 2

    if ($Config.Forge -eq 'gitea') {
        $result = Invoke-GiteaAPI -Path "/repos/$owner/$repo/issues?state=open&type=issues&limit=50" -Config $Config
        $issues = $result | Where-Object {
            $labels = $_.labels | ForEach-Object { $_.name }
            'status:queued' -in $labels -and ($labels | Where-Object { $_ -in $Config.AgentLabels })
        }
    } else {
        $json = Invoke-GitHubCLI -Args @(
            'issue', 'list',
            '--repo', $Config.Project,
            '--label', 'status:queued',
            '--state', 'open',
            '--json', 'number,title,body,labels,assignees',
            '--limit', '50'
        )
        $raw    = $json | ConvertFrom-Json
        $issues = $raw | Where-Object {
            $labelNames = $_.labels | ForEach-Object { $_.name }
            $labelNames | Where-Object { $_ -in $Config.AgentLabels }
        }
    }

    return $issues | ForEach-Object {
        $labelNames = $_.labels | ForEach-Object { $_.name }
        $agentLabel = $labelNames | Where-Object { $_ -in $Config.AgentLabels } | Select-Object -First 1
        $agentType  = if ($agentLabel) { $agentLabel -replace 'agent:', '' } else { 'unknown' }
        [PSCustomObject]@{
            Number    = $_.number
            Title     = $_.title
            Body      = $_.body
            Labels    = $labelNames
            AgentType = $agentType
        }
    }
}

function Get-IssueDetails {
    <#
    .SYNOPSIS
        Fetches the full issue body for a specific issue number.
    .PARAMETER IssueNumber
        Issue number to fetch.
    .PARAMETER Config
        Forge configuration hashtable.
    .OUTPUTS
        PSCustomObject with Number, Title, Body, Labels
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)][int]$IssueNumber,
        [hashtable]$Config = $script:DefaultForgeConfig
    )

    $owner, $repo = $Config.Project -split '/', 2

    if ($Config.Forge -eq 'gitea') {
        $result = Invoke-GiteaAPI -Path "/repos/$owner/$repo/issues/$IssueNumber" -Config $Config
        return [PSCustomObject]@{
            Number = $result.number
            Title  = $result.title
            Body   = $result.body
            Labels = $result.labels | ForEach-Object { $_.name }
        }
    } else {
        $json = Invoke-GitHubCLI -Args @(
            'issue', 'view',
            $IssueNumber,
            '--repo', $Config.Project,
            '--json', 'number,title,body,labels'
        )
        $raw = $json | ConvertFrom-Json
        return [PSCustomObject]@{
            Number = $raw.number
            Title  = $raw.title
            Body   = $raw.body
            Labels = $raw.labels | ForEach-Object { $_.name }
        }
    }
}

function Claim-Issue {
    <#
    .SYNOPSIS
        Claims an issue by swapping status:queued for status:claimed.
    .DESCRIPTION
        Attempts to claim the issue atomically by removing status:queued and
        adding status:claimed. If another agent has already claimed it (status:queued
        is gone), returns $false and the caller should skip this issue.
    .PARAMETER IssueNumber
        Issue number to claim.
    .PARAMETER Config
        Forge configuration hashtable.
    .OUTPUTS
        $true if claimed successfully; $false if already claimed by another agent.
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)][int]$IssueNumber,
        [hashtable]$Config = $script:DefaultForgeConfig
    )

    # Verify the issue still has status:queued (race condition check).
    $issue = Get-IssueDetails -IssueNumber $IssueNumber -Config $Config
    if ('status:queued' -notin $issue.Labels) {
        Write-Verbose "Issue #$IssueNumber no longer has status:queued — skipping (already claimed)."
        return $false
    }

    Update-IssueStatus -IssueNumber $IssueNumber `
        -AddLabels @('status:claimed') `
        -RemoveLabels @('status:queued') `
        -Config $Config

    Write-Host "Claimed issue #$IssueNumber"
    return $true
}

function Update-IssueStatus {
    <#
    .SYNOPSIS
        Adds and/or removes labels on an issue.
    .PARAMETER IssueNumber
        Issue number to update.
    .PARAMETER AddLabels
        Labels to add.
    .PARAMETER RemoveLabels
        Labels to remove.
    .PARAMETER Config
        Forge configuration hashtable.
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)][int]$IssueNumber,
        [string[]]$AddLabels    = @(),
        [string[]]$RemoveLabels = @(),
        [hashtable]$Config = $script:DefaultForgeConfig
    )

    $owner, $repo = $Config.Project -split '/', 2

    if ($Config.Forge -eq 'gitea') {
        if ($AddLabels.Count -gt 0) {
            # Gitea: first get label IDs, then POST /issues/{index}/labels
            $allLabels = Invoke-GiteaAPI -Path "/repos/$owner/$repo/labels?limit=50" -Config $Config
            $addIds = @($allLabels | Where-Object { $_.name -in $AddLabels } | ForEach-Object { $_.id })
            if ($addIds.Count -gt 0) {
                $null = Invoke-GiteaAPI -Path "/repos/$owner/$repo/issues/$IssueNumber/labels" `
                    -Method 'POST' `
                    -Body @{ labels = $addIds } `
                    -Config $Config
            }
        }
        foreach ($label in $RemoveLabels) {
            $allLabels = Invoke-GiteaAPI -Path "/repos/$owner/$repo/labels?limit=50" -Config $Config
            $labelObj = $allLabels | Where-Object { $_.name -eq $label } | Select-Object -First 1
            if ($labelObj) {
                $null = Invoke-GiteaAPI -Path "/repos/$owner/$repo/issues/$IssueNumber/labels/$($labelObj.id)" `
                    -Method 'DELETE' `
                    -Config $Config
            }
        }
    } else {
        if ($AddLabels.Count -gt 0) {
            $null = Invoke-GitHubCLI -Args (@('issue', 'edit', $IssueNumber, '--repo', $Config.Project) + ($AddLabels | ForEach-Object { '--add-label', $_ }))
        }
        if ($RemoveLabels.Count -gt 0) {
            $null = Invoke-GitHubCLI -Args (@('issue', 'edit', $IssueNumber, '--repo', $Config.Project) + ($RemoveLabels | ForEach-Object { '--remove-label', $_ }))
        }
    }
}

function Add-IssueComment {
    <#
    .SYNOPSIS
        Posts a comment on an issue.
    .PARAMETER IssueNumber
        Issue number to comment on.
    .PARAMETER Body
        Comment body text.
    .PARAMETER Config
        Forge configuration hashtable.
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)][int]$IssueNumber,
        [Parameter(Mandatory)][string]$Body,
        [hashtable]$Config = $script:DefaultForgeConfig
    )

    $owner, $repo = $Config.Project -split '/', 2

    if ($Config.Forge -eq 'gitea') {
        $null = Invoke-GiteaAPI -Path "/repos/$owner/$repo/issues/$IssueNumber/comments" `
            -Method 'POST' `
            -Body @{ body = $Body } `
            -Config $Config
    } else {
        $null = Invoke-GitHubCLI -Args @('issue', 'comment', $IssueNumber, '--repo', $Config.Project, '--body', $Body)
    }

    Write-Verbose "Posted comment on issue #$IssueNumber"
}

function Open-PullRequest {
    <#
    .SYNOPSIS
        Creates a pull request for a completed agent task.
    .PARAMETER IssueNumber
        Issue number being resolved (used in PR title and body).
    .PARAMETER Branch
        Head branch name (e.g. fix/42-add-feature).
    .PARAMETER Title
        PR title. Defaults to "fix(#<N>): <issue title>".
    .PARAMETER Config
        Forge configuration hashtable.
    .OUTPUTS
        PR number (int) or 0 on failure.
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)][int]$IssueNumber,
        [Parameter(Mandatory)][string]$Branch,
        [string]$Title = '',
        [hashtable]$Config = $script:DefaultForgeConfig
    )

    $owner, $repo = $Config.Project -split '/', 2

    if ($Title -eq '') {
        $issue = Get-IssueDetails -IssueNumber $IssueNumber -Config $Config
        $Title = "fix(#$IssueNumber): $($issue.Title)"
    }
    $body = "Closes #$IssueNumber`n`nAgent-generated implementation by $($Config.AgentId)."

    if ($Config.Forge -eq 'gitea') {
        $result = Invoke-GiteaAPI -Path "/repos/$owner/$repo/pulls" `
            -Method 'POST' `
            -Body @{ title = $Title; body = $body; head = $Branch; base = 'main' } `
            -Config $Config
        return $result.number
    } else {
        $json = Invoke-GitHubCLI -Args @(
            'pr', 'create',
            '--repo', $Config.Project,
            '--title', $Title,
            '--body', $body,
            '--head', $Branch,
            '--base', 'main'
        )
        # gh pr create returns the PR URL; extract number from last path segment.
        if ($json -match '/(\d+)$') {
            return [int]$Matches[1]
        }
        return 0
    }
}

Export-ModuleMember -Function @(
    'Get-ForgeConfig',
    'Get-ClaimableIssues',
    'Get-IssueDetails',
    'Claim-Issue',
    'Update-IssueStatus',
    'Add-IssueComment',
    'Open-PullRequest'
)
