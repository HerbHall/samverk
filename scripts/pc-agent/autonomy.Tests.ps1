#Requires -Version 7.0
<#
.SYNOPSIS
    Unit tests for autonomy.psm1 using Pester 5.
#>

BeforeAll {
    $moduleDir = Split-Path $PSCommandPath -Parent
    Import-Module (Join-Path $moduleDir 'autonomy.psm1') -Force
}

# ---------------------------------------------------------------------------
# Get-IssueTier
# ---------------------------------------------------------------------------

Describe 'Get-IssueTier' {
    It 'returns Tier 1 for complexity:local with normal priority' {
        Get-IssueTier -Labels @('complexity:local', 'priority:normal') | Should -Be 1
    }

    It 'returns Tier 1 for complexity:local with low priority' {
        Get-IssueTier -Labels @('complexity:local', 'priority:low') | Should -Be 1
    }

    It 'returns Tier 2 for complexity:cloud with normal priority' {
        Get-IssueTier -Labels @('complexity:cloud', 'priority:normal') | Should -Be 2
    }

    It 'returns Tier 2 for complexity:cloud with high priority' {
        Get-IssueTier -Labels @('complexity:cloud', 'priority:high') | Should -Be 2
    }

    It 'returns Tier 3 for complexity:ambiguous' {
        Get-IssueTier -Labels @('complexity:ambiguous', 'priority:normal') | Should -Be 3
    }

    It 'returns Tier 3 for priority:critical regardless of complexity' {
        Get-IssueTier -Labels @('complexity:local', 'priority:critical') | Should -Be 3
    }

    It 'returns Tier 3 when no complexity label is present' {
        Get-IssueTier -Labels @('priority:normal', 'type:task') | Should -Be 3
    }

    It 'returns Tier 3 for empty label list' {
        Get-IssueTier -Labels @() | Should -Be 3
    }
}

# ---------------------------------------------------------------------------
# Get-FrontmatterValue
# ---------------------------------------------------------------------------

Describe 'Get-FrontmatterValue' {
    BeforeAll {
        $script:bodyWithFrontmatter = @'
---
schema_version: "1.0.0"
type: "task"
depends_on: [306, 307]
priority: "high"
---
## Summary

This is the issue body.
'@

        $script:bodyScalar = @'
---
key: simple_value
---
Body text.
'@

        $script:bodyNoFrontmatter = 'No frontmatter here.'
    }

    It 'returns scalar value' {
        Get-FrontmatterValue -Body $script:bodyScalar -Key 'key' | Should -Be 'simple_value'
    }

    It 'returns list as array for inline list syntax' {
        $result = Get-FrontmatterValue -Body $script:bodyWithFrontmatter -Key 'depends_on'
        $result | Should -HaveCount 2
        $result[0] | Should -Be '306'
        $result[1] | Should -Be '307'
    }

    It 'returns $null for missing key' {
        Get-FrontmatterValue -Body $script:bodyWithFrontmatter -Key 'nonexistent' | Should -BeNullOrEmpty
    }

    It 'returns $null when body has no frontmatter' {
        Get-FrontmatterValue -Body $script:bodyNoFrontmatter -Key 'type' | Should -BeNullOrEmpty
    }
}

# ---------------------------------------------------------------------------
# Get-DependsOnNumbers
# ---------------------------------------------------------------------------

Describe 'Get-DependsOnNumbers' {
    It 'parses inline list of issue numbers' {
        $body = "---`nschema_version: `"1.0.0`"`ndepends_on: [10, 20, 30]`n---`nBody."
        $result = Get-DependsOnNumbers -Body $body
        $result | Should -HaveCount 3
        $result[0] | Should -Be 10
        $result[1] | Should -Be 20
        $result[2] | Should -Be 30
    }

    It 'returns empty array when depends_on is absent' {
        $body = "---`ntype: task`n---`nBody."
        $result = Get-DependsOnNumbers -Body $body
        @($result).Count | Should -Be 0
    }

    It 'returns empty array for body without frontmatter' {
        $result = Get-DependsOnNumbers -Body 'plain text'
        @($result).Count | Should -Be 0
    }

    It 'returns single number when depends_on has one entry' {
        $body = "---`ndepends_on: [42]`n---`nBody."
        $result = Get-DependsOnNumbers -Body $body
        @($result).Count | Should -Be 1
        $result[0] | Should -Be 42
    }
}

# ---------------------------------------------------------------------------
# Get-PriorFailureCount
# ---------------------------------------------------------------------------

Describe 'Get-PriorFailureCount' {
    It 'returns 0 when no failure label present' {
        Get-PriorFailureCount -Labels @('status:queued', 'complexity:local') | Should -Be 0
    }

    It 'returns failure count from pc-failure-N label' {
        Get-PriorFailureCount -Labels @('status:queued', 'pc-failure-2') | Should -Be 2
    }

    It 'returns first match when multiple failure labels present' {
        # Pathological case — only one should exist in practice.
        Get-PriorFailureCount -Labels @('pc-failure-1', 'pc-failure-3') | Should -BeIn @(1, 3)
    }

    It 'returns 0 for empty label list' {
        Get-PriorFailureCount -Labels @() | Should -Be 0
    }
}

# ---------------------------------------------------------------------------
# Test-AutonomyGate
# ---------------------------------------------------------------------------

Describe 'Test-AutonomyGate' {
    BeforeAll {
        # Minimal issue factory for gate tests.
        function New-TestIssue {
            param(
                [string[]]$Labels = @('complexity:local', 'priority:normal', 'status:queued', 'agent:code-gen'),
                [string]$Body = "---`ntype: task`n---`nBody."
            )
            return [PSCustomObject]@{
                Number    = 1
                Title     = 'Test issue'
                Body      = $Body
                Labels    = $Labels
                AgentType = 'code-gen'
            }
        }

        $script:DefaultCfg = @{
            MaxTier     = 2
            MaxFailures = 2
            SkipLabels  = @('agent:human', 'status:needs-human', 'agent:pc')
        }
    }

    It 'allows Tier 1 issue with default config' {
        $issue = New-TestIssue -Labels @('complexity:local', 'priority:normal', 'status:queued', 'agent:code-gen')
        $result = Test-AutonomyGate -Issue $issue -Config $script:DefaultCfg -SkipDependencyCheck
        $result.Allowed | Should -BeTrue
        $result.Tier | Should -Be 1
    }

    It 'allows Tier 2 issue with default config' {
        $issue = New-TestIssue -Labels @('complexity:cloud', 'priority:high', 'status:queued', 'agent:code-gen')
        $result = Test-AutonomyGate -Issue $issue -Config $script:DefaultCfg -SkipDependencyCheck
        $result.Allowed | Should -BeTrue
        $result.Tier | Should -Be 2
    }

    It 'blocks Tier 3 issue (ambiguous complexity)' {
        $issue = New-TestIssue -Labels @('complexity:ambiguous', 'priority:normal', 'status:queued')
        $result = Test-AutonomyGate -Issue $issue -Config $script:DefaultCfg -SkipDependencyCheck
        $result.Allowed | Should -BeFalse
        $result.Tier | Should -Be 3
        $result.Reason | Should -Match 'exceeds max'
    }

    It 'blocks issue with agent:human label' {
        $issue = New-TestIssue -Labels @('complexity:local', 'priority:normal', 'agent:human')
        $result = Test-AutonomyGate -Issue $issue -Config $script:DefaultCfg -SkipDependencyCheck
        $result.Allowed | Should -BeFalse
        $result.Tier | Should -Be 0
        $result.Reason | Should -Match 'agent:human'
    }

    It 'blocks issue with status:needs-human label' {
        $issue = New-TestIssue -Labels @('complexity:local', 'priority:normal', 'status:needs-human')
        $result = Test-AutonomyGate -Issue $issue -Config $script:DefaultCfg -SkipDependencyCheck
        $result.Allowed | Should -BeFalse
        $result.Reason | Should -Match 'needs-human'
    }

    It 'blocks issue with agent:pc label' {
        $issue = New-TestIssue -Labels @('complexity:local', 'priority:normal', 'agent:pc')
        $result = Test-AutonomyGate -Issue $issue -Config $script:DefaultCfg -SkipDependencyCheck
        $result.Allowed | Should -BeFalse
        $result.Tier | Should -Be 0
        $result.Reason | Should -Match 'agent:pc'
    }

    It 'blocks issue that has reached max failures' {
        $issue = New-TestIssue -Labels @('complexity:local', 'priority:normal', 'status:queued', 'pc-failure-2')
        $result = Test-AutonomyGate -Issue $issue -Config $script:DefaultCfg -SkipDependencyCheck
        $result.Allowed | Should -BeFalse
        $result.Reason | Should -Match 'failure'
    }

    It 'allows issue below max failure threshold' {
        $issue = New-TestIssue -Labels @('complexity:local', 'priority:normal', 'status:queued', 'pc-failure-1')
        $result = Test-AutonomyGate -Issue $issue -Config $script:DefaultCfg -SkipDependencyCheck
        $result.Allowed | Should -BeTrue
    }

    It 'blocks Tier 2 issue when MaxTier is 1' {
        $restrictedCfg = @{ MaxTier = 1; MaxFailures = 2; SkipLabels = @('agent:human') }
        $issue = New-TestIssue -Labels @('complexity:cloud', 'priority:normal', 'status:queued')
        $result = Test-AutonomyGate -Issue $issue -Config $restrictedCfg -SkipDependencyCheck
        $result.Allowed | Should -BeFalse
        $result.Reason | Should -Match 'exceeds max'
    }

    It 'skips dependency check when -SkipDependencyCheck is set' {
        $body = "---`ntype: task`ndepends_on: [999]`n---`nBody."
        $issue = New-TestIssue -Labels @('complexity:local', 'priority:normal', 'status:queued') -Body $body
        # Would fail dependency check if it ran (issue 999 is unlikely to exist).
        # With -SkipDependencyCheck, should pass.
        $result = Test-AutonomyGate -Issue $issue -Config $script:DefaultCfg -SkipDependencyCheck
        $result.Allowed | Should -BeTrue
    }
}
