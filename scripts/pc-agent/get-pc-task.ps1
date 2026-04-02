<#
.SYNOPSIS
    Discovers the next queued agent:pc issue, claims it, and renders a handoff
    prompt for pasting into a Claude Code session.

.DESCRIPTION
    Calls the Samverk REST API to find status:queued + agent:pc issues, picks
    the highest-priority one (critical > high > normal > oldest), claims it by
    swapping status:queued for status:claimed, and writes the handoff prompt to
    a temp file and the clipboard.

.PARAMETER ApiBase
    Base URL for the Samverk API. Defaults to http://192.168.1.162:8080/api/v1.

.EXAMPLE
    .\get-pc-task.ps1
    # Uses SAMVERK_AUTH_TOKEN env var, writes prompt to clipboard

.EXAMPLE
    .\get-pc-task.ps1 -ApiBase http://localhost:8080/api/v1
    # Local development server
#>
[CmdletBinding()]
param(
    [string]$ApiBase = "http://192.168.1.162:8080/api/v1"
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

# ---------------------------------------------------------------------------
# Forge config (from .samverk/project.yaml)
# ---------------------------------------------------------------------------
$scriptDir = Split-Path $PSCommandPath -Parent
Import-Module (Join-Path $scriptDir 'forge.psm1') -Force -WarningAction SilentlyContinue
$forgeConfig = Get-ForgeConfig

# ---------------------------------------------------------------------------
# Auth
# ---------------------------------------------------------------------------
$authToken = $env:SAMVERK_AUTH_TOKEN
if (-not $authToken) {
    Write-Error "SAMVERK_AUTH_TOKEN environment variable is not set."
    exit 1
}

$headers = @{
    "Authorization" = "Bearer $authToken"
    "Accept"        = "application/json"
}

# ---------------------------------------------------------------------------
# Helper: priority rank (lower = higher priority)
# ---------------------------------------------------------------------------
function Get-PriorityRank {
    param([string[]]$Labels)
    if ($Labels -contains "priority:critical") { return 0 }
    if ($Labels -contains "priority:high")     { return 1 }
    if ($Labels -contains "priority:normal")   { return 2 }
    return 3
}

# ---------------------------------------------------------------------------
# Helper: generate slug from title
# ---------------------------------------------------------------------------
function ConvertTo-Slug {
    param([string]$Title)
    $slug = $Title.ToLower()
    $slug = $slug -replace '[^a-z0-9 ]', ''
    $slug = $slug.Trim() -replace '\s+', '-'
    if ($slug.Length -gt 40) { $slug = $slug.Substring(0, 40).TrimEnd('-') }
    return $slug
}

# ---------------------------------------------------------------------------
# Helper: infer branch type from labels
# ---------------------------------------------------------------------------
function Get-BranchType {
    param([string[]]$Labels)
    if (($Labels -contains "fix") -or ($Labels -contains "bug")) { return "fix" }
    return "feat"
}

# ---------------------------------------------------------------------------
# Step 1: List queued agent:pc issues
# ---------------------------------------------------------------------------
Write-Host "Querying for agent:pc + status:queued issues..." -ForegroundColor Cyan

$uri = "$ApiBase/issues?labels=agent:pc,status:queued&state=open"
try {
    $response = Invoke-RestMethod -Uri $uri -Headers $headers -Method Get
} catch {
    Write-Error "Failed to query issues: $_"
    exit 1
}

if (-not $response -or $response.Count -eq 0) {
    Write-Host "No queued agent:pc issues found." -ForegroundColor Yellow
    exit 0
}

# ---------------------------------------------------------------------------
# Step 2: Pick highest priority (then oldest)
# ---------------------------------------------------------------------------
$sorted = $response | Sort-Object {
    Get-PriorityRank -Labels $_.labels
}, { [datetime]$_.created_at }

$issue = $sorted[0]
$issueNumber = $issue.number
$issueTitle  = $issue.title
$issueBody   = $issue.body
$labelNames  = @($issue.labels | ForEach-Object {
    if ($_ -is [string]) { $_ } else { $_.name }
})

Write-Host "Selected issue #$issueNumber`: $issueTitle" -ForegroundColor Green

# ---------------------------------------------------------------------------
# Step 3: Claim -- swap status:queued -> status:claimed
# ---------------------------------------------------------------------------
$newLabels = @($labelNames | Where-Object { $_ -ne "status:queued" })
if ($newLabels -notcontains "status:claimed") {
    $newLabels += "status:claimed"
}

$claimUri = "$ApiBase/issues/$issueNumber/labels"
$claimBody = @{ labels = $newLabels } | ConvertTo-Json -Compress
try {
    $null = Invoke-RestMethod -Uri $claimUri -Headers $headers -Method Put -Body $claimBody -ContentType "application/json"
} catch {
    Write-Error "Failed to claim issue #$issueNumber`: $_"
    exit 1
}

Write-Host "Claimed issue #$issueNumber (labels updated)." -ForegroundColor Green

# ---------------------------------------------------------------------------
# Step 4: Build handoff prompt
# ---------------------------------------------------------------------------
$branchType = Get-BranchType -Labels $labelNames
$slug       = ConvertTo-Slug -Title $issueTitle
$branch     = "$branchType/$issueNumber-$slug"

# Build the existing-labels-minus-claimed-plus-needs-qc list for the template
$completionLabels = @($newLabels | Where-Object { $_ -ne "status:claimed" })
if ($completionLabels -notcontains "status:needs-qc") {
    $completionLabels += "status:needs-qc"
}
$completionLabelStr = ($completionLabels | ForEach-Object { "`"$_`"" }) -join ", "

# Build the blocked labels list
$blockedLabels = @($newLabels | Where-Object { $_ -ne "status:claimed" })
if ($blockedLabels -notcontains "status:needs-human") {
    $blockedLabels += "status:needs-human"
}
$blockedLabelStr = ($blockedLabels | ForEach-Object { "`"$_`"" }) -join ", "

$prompt = @"
You are a Samverk PC agent. Implement issue #$issueNumber using the Samverk MCP tools
available in this VS Code session.

## Task
Title: $issueTitle
Branch: $branch
Issue: $($forgeConfig.ForgeUrl)/$($forgeConfig.Project)/issues/$issueNumber

## Issue Body
$issueBody

## Workspace Setup
Open a terminal and run:
  git -C D:\bots\samverk.git fetch --all
  git worktree add D:\bots\worker-1 -b $branch origin/main
  cd D:\bots\worker-1

(If worker-1 is in use, pick the next available slot: worker-2, worker-3)

## Implementation
- Read CLAUDE.md for conventions and build commands.
- Implement all acceptance criteria in the issue body.
- Run ``make ci`` before committing. Fix all failures.
- Commit: ``git commit -m "$branchType(#$issueNumber): <summary>"``

## Completion (after make ci passes and changes are committed)

Run in terminal:
  git push origin $branch

Then via Samverk MCP tools in this session:

1. Create PR:
   create_pr(title="$branchType(#$issueNumber): $issueTitle", head="$branch", base="main",
             body="Closes #$issueNumber\n\n<one-line summary of changes>")
   Note the PR number returned.

2. Update issue labels:
   set_labels(issue_number=$issueNumber, labels=[$completionLabelStr])

3. Post completion comment:
   add_comment(issue_number=$issueNumber,
               body="PR opened: #<PR number> -- $branch")

## If Blocked
Create AGENT_BLOCKED.md in the project root with:
- What you attempted
- What is blocking you
- What would unblock you

Then via Samverk MCP:
  set_labels(issue_number=$issueNumber, labels=[$blockedLabelStr])

Do NOT commit incomplete work.
"@

# ---------------------------------------------------------------------------
# Step 5: Write to file and clipboard
# ---------------------------------------------------------------------------
$promptFile = Join-Path $env:TEMP "pc-task-prompt.md"
$prompt | Out-File -FilePath $promptFile -Encoding utf8

Set-Clipboard -Value $prompt

Write-Host ""
Write-Host "Claimed issue #$issueNumber`: $issueTitle. Prompt copied to clipboard." -ForegroundColor Green
Write-Host "Prompt also saved to: $promptFile" -ForegroundColor Gray
