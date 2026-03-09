#Requires -Version 7.0
<#
.SYNOPSIS
    PC Agent registration and heartbeat -- keeps the Samverk server aware of this
    worker node and its live metrics.

.DESCRIPTION
    Provides three public functions:

        Register-PCAgent       -- POST /api/v1/workers/register on startup
        Send-Heartbeat         -- POST /api/v1/workers/heartbeat with live metrics
        Start-HeartbeatLoop    -- Starts a background thread that calls Send-Heartbeat
                                  every HeartbeatIntervalSeconds (default 150s) until
                                  Stop-HeartbeatLoop is called
        Stop-HeartbeatLoop     -- Stops the background heartbeat thread

    Configuration via environment variables:

        SAMVERK_SERVER_URL     Base URL of the Samverk server  (default: http://localhost:8080)
        SAMVERK_AUTH_TOKEN     Bearer token for API auth       (optional)

    The agent_id is derived from the machine hostname. It must be stable across
    restarts so the server can correlate heartbeats with the registration record.
#>

Set-StrictMode -Version 3.0
$ErrorActionPreference = 'Stop'

# ---------------------------------------------------------------------------
# Module-level state
# ---------------------------------------------------------------------------

$script:HeartbeatJob = $null

# ---------------------------------------------------------------------------
# Internal helpers
# ---------------------------------------------------------------------------

function Get-ServerURL {
    $url = $env:SAMVERK_SERVER_URL
    if (-not $url) { $url = 'http://localhost:8080' }
    return $url.TrimEnd('/')
}

function Get-AuthHeaders {
    $headers = @{ 'Content-Type' = 'application/json' }
    if ($env:SAMVERK_AUTH_TOKEN) {
        $headers['Authorization'] = "Bearer $($env:SAMVERK_AUTH_TOKEN)"
    }
    return $headers
}

function Get-AgentID {
    # Use the machine hostname as a stable, human-readable agent identifier.
    return $env:COMPUTERNAME.ToLower()
}

function Get-CPUPercent {
    try {
        $sample = Get-CimInstance Win32_Processor -ErrorAction SilentlyContinue |
            Measure-Object -Property LoadPercentage -Average
        if ($null -ne $sample -and $null -ne $sample.Average) {
            return [double]$sample.Average
        }
    } catch {
        # Non-fatal -- return 0 on error.
    }
    return 0.0
}

function Get-MemoryPercent {
    try {
        $os = Get-CimInstance Win32_OperatingSystem -ErrorAction SilentlyContinue
        if ($os -and $os.TotalVisibleMemorySize -gt 0) {
            $used = $os.TotalVisibleMemorySize - $os.FreePhysicalMemory
            return [Math]::Round(($used / $os.TotalVisibleMemorySize) * 100.0, 1)
        }
    } catch {
        # Non-fatal.
    }
    return 0.0
}

function Get-ActiveWorktreeCount {
    param([string]$WorkspaceRoot = 'D:\bots')
    try {
        $count = (Get-ChildItem -Path $WorkspaceRoot -Directory -Filter 'worker-*' -ErrorAction SilentlyContinue).Count
        return [int]$count
    } catch {
        return 0
    }
}

function Invoke-RegistrationRequest {
    param(
        [string]$Path,
        [hashtable]$Body
    )
    $url     = (Get-ServerURL) + $Path
    $headers = Get-AuthHeaders
    $json    = $Body | ConvertTo-Json -Compress -Depth 5
    $bytes   = [System.Text.Encoding]::UTF8.GetBytes($json)

    try {
        $resp = Invoke-WebRequest -Uri $url -Method POST -Headers $headers `
            -Body $bytes -ContentType 'application/json' `
            -TimeoutSec 10 -UseBasicParsing -ErrorAction Stop
        return $resp.StatusCode
    } catch [System.Net.WebException] {
        $statusCode = [int]$_.Exception.Response.StatusCode
        throw "HTTP $statusCode from $url : $($_.Exception.Message)"
    }
}

# ---------------------------------------------------------------------------
# Public functions
# ---------------------------------------------------------------------------

function Register-PCAgent {
    <#
    .SYNOPSIS
        Registers this PC agent with the Samverk server. Call once on startup.

    .PARAMETER Capabilities
        List of capability tags sent to the server (e.g. 'codegen', 'go', 'react').
        Defaults to a generic set.

    .PARAMETER MaxConcurrent
        Maximum number of concurrent tasks this agent supports. Default: 1.

    .PARAMETER WorkspaceRoot
        Root directory containing worker-* worktrees. Default: D:\bots.
    #>
    [CmdletBinding()]
    param(
        [string[]]$Capabilities = @('codegen', 'go', 'react', 'windows'),
        [int]$MaxConcurrent = 1,
        [string]$WorkspaceRoot = 'D:\bots'
    )

    $body = @{
        agent_id       = Get-AgentID
        hostname       = $env:COMPUTERNAME
        capabilities   = $Capabilities
        max_concurrent = $MaxConcurrent
        workspace_root = $WorkspaceRoot
    }

    $status = Invoke-RegistrationRequest -Path '/api/v1/workers/register' -Body $body
    Write-Host "[registration] Registered with Samverk server (HTTP $status). agent_id=$(Get-AgentID)"
}

function Send-Heartbeat {
    <#
    .SYNOPSIS
        Sends a single heartbeat to the Samverk server with live worker metrics.

    .PARAMETER CurrentTask
        Issue number of the task currently being processed. Omit when idle.

    .PARAMETER Status
        Worker status string: 'idle' or 'busy'. Defaults to 'idle'.

    .PARAMETER WorkspaceRoot
        Root directory containing worker-* worktrees. Default: D:\bots.
    #>
    [CmdletBinding()]
    param(
        [Nullable[int]]$CurrentTask = $null,
        [string]$Status = 'idle',
        [string]$WorkspaceRoot = 'D:\bots'
    )

    $body = @{
        agent_id         = Get-AgentID
        status           = $Status
        cpu_percent      = Get-CPUPercent
        memory_percent   = Get-MemoryPercent
        active_worktrees = Get-ActiveWorktreeCount -WorkspaceRoot $WorkspaceRoot
    }

    if ($null -ne $CurrentTask) {
        $body['current_task'] = $CurrentTask
    }

    Invoke-RegistrationRequest -Path '/api/v1/workers/heartbeat' -Body $body | Out-Null
}

function Start-HeartbeatLoop {
    <#
    .SYNOPSIS
        Starts a background thread that sends heartbeats on a fixed interval.

    .DESCRIPTION
        Runs a thread job (Start-ThreadJob) that calls Send-Heartbeat every
        HeartbeatIntervalSeconds. The thread is stored in $script:HeartbeatJob.
        Call Stop-HeartbeatLoop to terminate it cleanly.

        The shared variable $script:CurrentTask should be set by the main loop
        before each task and cleared (set to $null) after the task completes.
        The heartbeat thread reads this value on each cycle.

    .PARAMETER HeartbeatIntervalSeconds
        Seconds between heartbeat calls. Default: 150 (2.5 minutes), which is
        safely within the 5-minute server-side stale threshold.

    .PARAMETER WorkspaceRoot
        Root directory containing worker-* worktrees. Default: D:\bots.
    #>
    [CmdletBinding()]
    param(
        [int]$HeartbeatIntervalSeconds = 150,
        [string]$WorkspaceRoot = 'D:\bots'
    )

    if ($script:HeartbeatJob -and $script:HeartbeatJob.State -eq 'Running') {
        Write-Host "[registration] Heartbeat loop already running."
        return
    }

    # Capture values for the thread closure.
    $interval      = $HeartbeatIntervalSeconds
    $wsRoot        = $WorkspaceRoot
    $serverURL     = Get-ServerURL
    $authToken     = $env:SAMVERK_AUTH_TOKEN
    $agentID       = Get-AgentID
    $hostname      = $env:COMPUTERNAME

    $script:HeartbeatJob = Start-ThreadJob -ScriptBlock {
        param($interval, $wsRoot, $serverURL, $authToken, $agentID, $hostname)

        function hb-cpu {
            try {
                $s = Get-CimInstance Win32_Processor -ErrorAction SilentlyContinue |
                    Measure-Object -Property LoadPercentage -Average
                if ($s -and $s.Average) { return [double]$s.Average }
            } catch {}
            return 0.0
        }

        function hb-mem {
            try {
                $os = Get-CimInstance Win32_OperatingSystem -ErrorAction SilentlyContinue
                if ($os -and $os.TotalVisibleMemorySize -gt 0) {
                    $used = $os.TotalVisibleMemorySize - $os.FreePhysicalMemory
                    return [Math]::Round(($used / $os.TotalVisibleMemorySize) * 100.0, 1)
                }
            } catch {}
            return 0.0
        }

        function hb-worktrees {
            try {
                return [int](Get-ChildItem -Path $wsRoot -Directory -Filter 'worker-*' -ErrorAction SilentlyContinue).Count
            } catch { return 0 }
        }

        function hb-send {
            param($status, $currentTask)
            $body = @{
                agent_id         = $agentID
                status           = $status
                cpu_percent      = hb-cpu
                memory_percent   = hb-mem
                active_worktrees = hb-worktrees
            }
            if ($null -ne $currentTask) { $body['current_task'] = $currentTask }

            $json  = $body | ConvertTo-Json -Compress -Depth 3
            $bytes = [System.Text.Encoding]::UTF8.GetBytes($json)
            $hdrs  = @{ 'Content-Type' = 'application/json' }
            if ($authToken) { $hdrs['Authorization'] = "Bearer $authToken" }

            try {
                $null = Invoke-WebRequest -Uri "$serverURL/api/v1/workers/heartbeat" `
                    -Method POST -Headers $hdrs -Body $bytes `
                    -TimeoutSec 10 -UseBasicParsing -ErrorAction Stop
            } catch {
                # Heartbeat failure is non-fatal -- log and continue.
                Write-Host "[heartbeat] Warning: $($_.Exception.Message)"
            }
        }

        while ($true) {
            Start-Sleep -Seconds $interval
            # Read the shared current-task variable from the parent runspace (best-effort).
            try {
                $ct = $using:script:CurrentTask
                $st = if ($null -ne $ct) { 'busy' } else { 'idle' }
                hb-send -status $st -currentTask $ct
            } catch {
                hb-send -status 'idle' -currentTask $null
            }
        }
    } -ArgumentList $interval, $wsRoot, $serverURL, $authToken, $agentID, $hostname

    Write-Host "[registration] Heartbeat loop started (interval=${interval}s)."
}

function Stop-HeartbeatLoop {
    <#
    .SYNOPSIS
        Stops the background heartbeat thread started by Start-HeartbeatLoop.
    #>
    [CmdletBinding()]
    param()

    if (-not $script:HeartbeatJob) { return }

    Stop-Job  -Job $script:HeartbeatJob -ErrorAction SilentlyContinue
    Remove-Job -Job $script:HeartbeatJob -Force -ErrorAction SilentlyContinue
    $script:HeartbeatJob = $null
    Write-Host "[registration] Heartbeat loop stopped."
}

Export-ModuleMember -Function Register-PCAgent, Send-Heartbeat, Start-HeartbeatLoop, Stop-HeartbeatLoop
