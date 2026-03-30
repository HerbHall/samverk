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
    # $env:COMPUTERNAME is not set on Linux; fall back to the hostname command.
    $name = $env:COMPUTERNAME
    if (-not $name) {
        $name = (hostname).Trim()
    }
    return $name.ToLower()
}

function Get-CPUPercent {
    if ($IsLinux) {
        try {
            # Read /proc/stat twice with a 1-second interval to compute CPU%.
            $line1 = (Get-Content -Path '/proc/stat' -TotalCount 1) -split '\s+'
            Start-Sleep -Seconds 1
            $line2 = (Get-Content -Path '/proc/stat' -TotalCount 1) -split '\s+'

            # Fields: cpu user nice system idle iowait irq softirq steal
            $idle1  = [long]$line1[4] + [long]$line1[5]  # idle + iowait
            $idle2  = [long]$line2[4] + [long]$line2[5]
            $total1 = [long]0
            $total2 = [long]0
            for ($i = 1; $i -le 8; $i++) {
                $total1 += [long]$line1[$i]
                $total2 += [long]$line2[$i]
            }

            $deltaTotal = $total2 - $total1
            $deltaIdle  = $idle2 - $idle1
            if ($deltaTotal -gt 0) {
                return [Math]::Round((1.0 - ($deltaIdle / $deltaTotal)) * 100.0, 1)
            }
        } catch {
            # Non-fatal -- fall through to return 0.
        }
        return 0.0
    }

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
    if ($IsLinux) {
        try {
            $meminfo = Get-Content -Path '/proc/meminfo' -TotalCount 8
            $total     = 0
            $available = 0
            foreach ($line in $meminfo) {
                if ($line -match '^MemTotal:\s+(\d+)') {
                    $total = [long]$Matches[1]
                }
                if ($line -match '^MemAvailable:\s+(\d+)') {
                    $available = [long]$Matches[1]
                }
            }
            if ($total -gt 0) {
                $used = $total - $available
                return [Math]::Round(($used / $total) * 100.0, 1)
            }
        } catch {
            # Non-fatal -- fall through to return 0.
        }
        return 0.0
    }

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
        [string[]]$Capabilities = @('codegen', 'go', 'react', $(if ($IsLinux) { 'linux' } else { 'windows' })),
        [int]$MaxConcurrent = 1,
        [string]$WorkspaceRoot = 'D:\bots'
    )

    $agentID = Get-AgentID

    $body = @{
        agent_id       = $agentID
        hostname       = $agentID
        capabilities   = $Capabilities
        max_concurrent = $MaxConcurrent
        workspace_root = $WorkspaceRoot
    }

    $status = Invoke-RegistrationRequest -Path '/api/v1/workers/register' -Body $body
    Write-Host "[registration] Registered with Samverk server (HTTP $status). agent_id=$agentID"
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
    $isLinuxHost   = [bool]$IsLinux

    $script:HeartbeatJob = Start-ThreadJob -ScriptBlock {
        param($interval, $wsRoot, $serverURL, $authToken, $agentID, $isLinuxHost)

        function hb-cpu {
            if ($isLinuxHost) {
                try {
                    $line1 = (Get-Content -Path '/proc/stat' -TotalCount 1) -split '\s+'
                    Start-Sleep -Seconds 1
                    $line2 = (Get-Content -Path '/proc/stat' -TotalCount 1) -split '\s+'

                    $idle1  = [long]$line1[4] + [long]$line1[5]
                    $idle2  = [long]$line2[4] + [long]$line2[5]
                    $total1 = [long]0
                    $total2 = [long]0
                    for ($i = 1; $i -le 8; $i++) {
                        $total1 += [long]$line1[$i]
                        $total2 += [long]$line2[$i]
                    }

                    $deltaTotal = $total2 - $total1
                    $deltaIdle  = $idle2 - $idle1
                    if ($deltaTotal -gt 0) {
                        return [Math]::Round((1.0 - ($deltaIdle / $deltaTotal)) * 100.0, 1)
                    }
                } catch {}
                return 0.0
            }

            try {
                $s = Get-CimInstance Win32_Processor -ErrorAction SilentlyContinue |
                    Measure-Object -Property LoadPercentage -Average
                if ($s -and $s.Average) { return [double]$s.Average }
            } catch {}
            return 0.0
        }

        function hb-mem {
            if ($isLinuxHost) {
                try {
                    $meminfo = Get-Content -Path '/proc/meminfo' -TotalCount 8
                    $total     = 0
                    $available = 0
                    foreach ($line in $meminfo) {
                        if ($line -match '^MemTotal:\s+(\d+)') {
                            $total = [long]$Matches[1]
                        }
                        if ($line -match '^MemAvailable:\s+(\d+)') {
                            $available = [long]$Matches[1]
                        }
                    }
                    if ($total -gt 0) {
                        $used = $total - $available
                        return [Math]::Round(($used / $total) * 100.0, 1)
                    }
                } catch {}
                return 0.0
            }

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
    } -ArgumentList $interval, $wsRoot, $serverURL, $authToken, $agentID, $isLinuxHost

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
