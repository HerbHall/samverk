#Requires -Version 7.0
<#
.SYNOPSIS
    PC Agent workspace infrastructure — bare clone and worktree lifecycle management.

.DESCRIPTION
    Provides functions to manage an isolated agent workspace under a configurable
    root directory (default D:\bots\). Agents NEVER access the user's working copy.

    Directory layout:
        <Root>\samverk.git\    ← Bare clone (shared object store)
        <Root>\worker-1\       ← Worktree: agent session 1
        <Root>\worker-2\       ← Worktree: agent session 2
        <Root>\worker-3\       ← Worktree: agent session 3

    CRITICAL INVARIANT: No function in this module reads from or writes to
    the user's working copy. All agent file operations occur within worktree
    directories under the workspace root.
#>

Set-StrictMode -Version 3.0
$ErrorActionPreference = 'Stop'

# Default configuration — callers can override by passing -Config.
# Clone URL is derived from .samverk/project.yaml at runtime.
$script:DefaultConfig = @{
    Root         = 'D:\bots'
    BareRepo     = 'samverk.git'
    MaxWorktrees = 3
    CloneUrl     = ''
}

function Get-WorkspaceConfig {
    <#
    .SYNOPSIS
        Returns workspace configuration, deriving clone URL from .samverk/project.yaml.
    .DESCRIPTION
        Reads forge_url from project.yaml and appends .git to form the clone URL.
        Falls back to SAMVERK_FORGE_URL environment variable if project.yaml is
        not found.
    .PARAMETER YamlPath
        Path to project.yaml. If omitted, auto-detected from repo root.
    #>
    [CmdletBinding()]
    param(
        [string]$YamlPath = ''
    )

    $config = $script:DefaultConfig.Clone()

    # Import Read-ProjectConfig from forge.psm1 if available.
    if (-not (Get-Command Read-ProjectConfig -ErrorAction SilentlyContinue)) {
        Import-Module (Join-Path $PSScriptRoot 'forge.psm1') -Global -WarningAction SilentlyContinue
    }

    $projectConfig = Read-ProjectConfig -Path $YamlPath
    if ($projectConfig.forge_url) {
        $config.CloneUrl = "$($projectConfig.forge_url).git"
    } elseif ($env:SAMVERK_FORGE_URL -and $env:SAMVERK_FORGE_PROJECT) {
        $config.CloneUrl = "$env:SAMVERK_FORGE_URL/$env:SAMVERK_FORGE_PROJECT.git"
    }

    return $config
}

function Get-BareRepoPath {
    param([hashtable]$Config = $script:DefaultConfig)
    return Join-Path $Config.Root $Config.BareRepo
}

function Initialize-AgentWorkspace {
    <#
    .SYNOPSIS
        One-time setup: creates the bare clone at the configured location.
    .DESCRIPTION
        Clones the primary remote (github) as a bare repository. Adds the
        secondary remote (gitea) so Update-BareRepo can push to both.
        Safe to call multiple times — skips setup if the bare repo already exists.
    .PARAMETER Config
        Workspace configuration hashtable.
    .PARAMETER Force
        Remove and re-create the bare repo if it already exists.
    #>
    [CmdletBinding()]
    param(
        [hashtable]$Config = $script:DefaultConfig,
        [switch]$Force
    )

    $bareRepo = Get-BareRepoPath -Config $Config

    if (Test-Path $bareRepo) {
        if ($Force) {
            Write-Verbose "Force: removing existing bare repo at $bareRepo"
            Remove-Item -Recurse -Force $bareRepo
        } else {
            Write-Host "Bare repo already exists at $bareRepo — skipping clone."
            return
        }
    }

    if (-not $Config.CloneUrl) {
        throw "CloneUrl not configured. Ensure .samverk/project.yaml exists with forge_url, or set SAMVERK_FORGE_URL + SAMVERK_FORGE_PROJECT."
    }

    $null = New-Item -ItemType Directory -Force -Path $Config.Root

    Write-Host "Cloning bare repo from $($Config.CloneUrl) to $bareRepo ..."
    $null = & git clone --bare $Config.CloneUrl $bareRepo 2>&1
    if ($LASTEXITCODE -ne 0) {
        throw "git clone --bare failed (exit $LASTEXITCODE)"
    }

    Write-Host "Workspace initialized at $($Config.Root)"
}

function Update-BareRepo {
    <#
    .SYNOPSIS
        Fetches the latest commits from all remotes into the bare clone.
    .DESCRIPTION
        Should be called before creating a new worktree to ensure the agent
        starts from the latest main branch.
    .PARAMETER Config
        Workspace configuration hashtable.
    #>
    [CmdletBinding()]
    param(
        [hashtable]$Config = $script:DefaultConfig
    )

    $bareRepo = Get-BareRepoPath -Config $Config
    if (-not (Test-Path $bareRepo)) {
        throw "Bare repo not found at $bareRepo — run Initialize-AgentWorkspace first."
    }

    Write-Verbose "Fetching from origin (github) ..."
    $null = & git -C $bareRepo fetch --all --prune 2>&1
    if ($LASTEXITCODE -ne 0) {
        throw "git fetch failed (exit $LASTEXITCODE)"
    }

    Write-Verbose "Bare repo updated."
}

function Get-AvailableWorkerSlot {
    <#
    .SYNOPSIS
        Returns the name of the next available worker slot (e.g. "worker-2").
    .DESCRIPTION
        Scans the workspace root for existing worker directories and returns the
        lowest-numbered slot that is not currently in use.
        Returns $null when all slots are occupied.
    .PARAMETER Config
        Workspace configuration hashtable.
    #>
    [CmdletBinding()]
    param(
        [hashtable]$Config = $script:DefaultConfig
    )

    for ($i = 1; $i -le $Config.MaxWorktrees; $i++) {
        $slot = "worker-$i"
        $path = Join-Path $Config.Root $slot
        if (-not (Test-Path $path)) {
            return $slot
        }
    }
    return $null
}

function New-AgentWorktree {
    <#
    .SYNOPSIS
        Creates an isolated worktree for a single agent task.
    .DESCRIPTION
        Provisions a git worktree on a new branch named fix/<issueNumber>-<slug>.
        The worktree contains the full project (CLAUDE.md, Makefile, go.mod, etc.)
        and is isolated from the user's working copy.
    .PARAMETER IssueNumber
        The GitHub/Gitea issue number being worked on.
    .PARAMETER IssueTitle
        The issue title, used to build the branch slug.
    .PARAMETER Slot
        Worker slot name (e.g. "worker-1"). If omitted, the next available slot is used.
    .PARAMETER Config
        Workspace configuration hashtable.
    .OUTPUTS
        Hashtable with keys: Slot, Path, Branch, BareRepo
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)]
        [int]$IssueNumber,
        [Parameter(Mandatory)]
        [string]$IssueTitle,
        [string]$Slot = '',
        [hashtable]$Config = $script:DefaultConfig
    )

    $bareRepo = Get-BareRepoPath -Config $Config
    if (-not (Test-Path $bareRepo)) {
        throw "Bare repo not found at $bareRepo — run Initialize-AgentWorkspace first."
    }

    if ($Slot -eq '') {
        $Slot = Get-AvailableWorkerSlot -Config $Config
        if ($null -eq $Slot) {
            throw "All $($Config.MaxWorktrees) worker slots are occupied. Wait for a task to complete."
        }
    }

    $worktreePath = Join-Path $Config.Root $Slot
    if (Test-Path $worktreePath) {
        throw "Slot $Slot is already in use at $worktreePath."
    }

    # Build branch slug: lowercase, alphanumeric + hyphens, max 40 chars.
    $slug = ($IssueTitle -replace '[^a-zA-Z0-9\s]', '' -replace '\s+', '-').ToLower()
    $slug = $slug.Substring(0, [Math]::Min($slug.Length, 40)).TrimEnd('-')
    $branch = "fix/$IssueNumber-$slug"

    # Fetch latest from origin before creating worktree.
    $null = & git -C $bareRepo fetch origin main:main --force 2>&1

    Write-Host "Creating worktree: slot=$Slot branch=$branch path=$worktreePath"
    $null = & git -C $bareRepo worktree add $worktreePath -b $branch main 2>&1
    if ($LASTEXITCODE -ne 0) {
        throw "git worktree add failed (exit $LASTEXITCODE). Branch '$branch' may already exist."
    }

    return @{
        Slot     = $Slot
        Path     = $worktreePath
        Branch   = $branch
        BareRepo = $bareRepo
    }
}

function Remove-AgentWorktree {
    <#
    .SYNOPSIS
        Removes a worktree and cleans up dangling refs.
    .DESCRIPTION
        Deletes the worktree directory and prunes the worktree list from the
        bare repo. Safe to call even if the worktree was already removed.
    .PARAMETER Slot
        Worker slot name (e.g. "worker-1").
    .PARAMETER Config
        Workspace configuration hashtable.
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)]
        [string]$Slot,
        [hashtable]$Config = $script:DefaultConfig
    )

    $bareRepo  = Get-BareRepoPath -Config $Config
    $worktreePath = Join-Path $Config.Root $Slot

    if (Test-Path $worktreePath) {
        Write-Verbose "Removing worktree at $worktreePath ..."
        $null = & git -C $bareRepo worktree remove --force $worktreePath 2>&1
        if ($LASTEXITCODE -ne 0) {
            # Fallback: manual directory removal if git worktree remove fails.
            Write-Warning "git worktree remove failed — removing directory manually."
            Remove-Item -Recurse -Force $worktreePath -ErrorAction SilentlyContinue
        }
    } else {
        Write-Verbose "Worktree path $worktreePath does not exist — skipping removal."
    }

    # Prune stale worktree metadata from the bare repo.
    $null = & git -C $bareRepo worktree prune 2>&1
    Write-Verbose "Slot $Slot cleaned up."
}

function Test-WorktreeHealth {
    <#
    .SYNOPSIS
        Verifies that a worktree is in a clean, functional state.
    .DESCRIPTION
        Checks that the worktree path exists, git status is clean (no uncommitted
        changes), and go.mod is present (project structure intact).
    .PARAMETER Slot
        Worker slot name (e.g. "worker-1").
    .PARAMETER Config
        Workspace configuration hashtable.
    .OUTPUTS
        $true if healthy, $false with a warning message if not.
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)]
        [string]$Slot,
        [hashtable]$Config = $script:DefaultConfig
    )

    $worktreePath = Join-Path $Config.Root $Slot

    if (-not (Test-Path $worktreePath)) {
        Write-Warning "Worktree path does not exist: $worktreePath"
        return $false
    }

    if (-not (Test-Path (Join-Path $worktreePath 'go.mod'))) {
        Write-Warning "go.mod not found in worktree — project structure may be incomplete."
        return $false
    }

    $status = & git -C $worktreePath status --porcelain 2>&1 | Out-String
    if ($LASTEXITCODE -ne 0) {
        Write-Warning "git status failed in worktree $worktreePath"
        return $false
    }

    if ($status.Trim() -ne '') {
        Write-Warning "Worktree has uncommitted changes:`n$status"
        return $false
    }

    return $true
}

Export-ModuleMember -Function @(
    'Get-WorkspaceConfig',
    'Initialize-AgentWorkspace',
    'Update-BareRepo',
    'Get-AvailableWorkerSlot',
    'New-AgentWorktree',
    'Remove-AgentWorktree',
    'Test-WorktreeHealth'
)
