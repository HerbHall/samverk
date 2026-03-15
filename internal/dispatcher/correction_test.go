package dispatcher

import (
	"testing"
	"time"

	"github.com/herbhall/samverk/pkg/models"
)

func TestDecideCorrection(t *testing.T) {
	const issueNum = 42
	const timeout = 5 * time.Minute

	tests := []struct {
		name           string
		failureClass   models.FailureClass
		attempt        int
		wantAction     CorrectionAction
		wantScope      CorrectionScope
		wantHasBackoff bool
	}{
		// ProviderDown: always switch provider.
		{
			name:           "provider_down/attempt_1",
			failureClass:   models.FailureClassProviderDown,
			attempt:        1,
			wantAction:     CorrectionActionSwitchProvider,
			wantScope:      CorrectionScopeTemporary,
			wantHasBackoff: true,
		},
		{
			name:           "provider_down/attempt_3",
			failureClass:   models.FailureClassProviderDown,
			attempt:        3,
			wantAction:     CorrectionActionSwitchProvider,
			wantScope:      CorrectionScopeTemporary,
			wantHasBackoff: true,
		},

		// Timeout: increase timeout, then escalate.
		{
			name:           "timeout/attempt_1",
			failureClass:   models.FailureClassTimeout,
			attempt:        1,
			wantAction:     CorrectionActionIncreaseTimeout,
			wantScope:      CorrectionScopeTemporary,
			wantHasBackoff: true,
		},
		{
			name:           "timeout/attempt_2",
			failureClass:   models.FailureClassTimeout,
			attempt:        2,
			wantAction:     CorrectionActionIncreaseTimeout,
			wantScope:      CorrectionScopeTemporary,
			wantHasBackoff: true,
		},
		{
			name:       "timeout/attempt_3_escalates",
			failureClass: models.FailureClassTimeout,
			attempt:    3,
			wantAction: CorrectionActionEscalate,
			wantScope:  CorrectionScopePermanent,
		},

		// Auth: always escalate immediately.
		{
			name:       "auth/attempt_1",
			failureClass: models.FailureClassAuth,
			attempt:    1,
			wantAction: CorrectionActionEscalate,
			wantScope:  CorrectionScopePermanent,
		},

		// Permanent: always escalate.
		{
			name:       "permanent/attempt_1",
			failureClass: models.FailureClassPermanent,
			attempt:    1,
			wantAction: CorrectionActionEscalate,
			wantScope:  CorrectionScopePermanent,
		},

		// Budget: pause and escalate.
		{
			name:       "budget/attempt_1",
			failureClass: models.FailureClassBudget,
			attempt:    1,
			wantAction: CorrectionActionPause,
			wantScope:  CorrectionScopePermanent,
		},

		// OOMKill: route to higher resources.
		{
			name:           "oom_kill/attempt_1",
			failureClass:   models.FailureClassOOMKill,
			attempt:        1,
			wantAction:     CorrectionActionRouteHigher,
			wantScope:      CorrectionScopeTemporary,
			wantHasBackoff: true,
		},

		// Panic: retry with context, then escalate.
		{
			name:           "panic/attempt_1",
			failureClass:   models.FailureClassPanic,
			attempt:        1,
			wantAction:     CorrectionActionRetryWithContext,
			wantScope:      CorrectionScopeTemporary,
			wantHasBackoff: true,
		},
		{
			name:       "panic/attempt_3_escalates",
			failureClass: models.FailureClassPanic,
			attempt:    3,
			wantAction: CorrectionActionEscalate,
			wantScope:  CorrectionScopePermanent,
		},

		// PostProcess: retry, then escalate.
		{
			name:           "post_process/attempt_1",
			failureClass:   models.FailureClassPostProcess,
			attempt:        1,
			wantAction:     CorrectionActionRetry,
			wantScope:      CorrectionScopeTemporary,
			wantHasBackoff: true,
		},
		{
			name:       "post_process/attempt_3_escalates",
			failureClass: models.FailureClassPostProcess,
			attempt:    3,
			wantAction: CorrectionActionEscalate,
			wantScope:  CorrectionScopePermanent,
		},

		// Classify: always escalate.
		{
			name:       "classify/attempt_1",
			failureClass: models.FailureClassClassify,
			attempt:    1,
			wantAction: CorrectionActionEscalate,
			wantScope:  CorrectionScopePermanent,
		},

		// Cycle: always escalate.
		{
			name:       "cycle/attempt_1",
			failureClass: models.FailureClassCycle,
			attempt:    1,
			wantAction: CorrectionActionEscalate,
			wantScope:  CorrectionScopePermanent,
		},

		// Decompose: retry, then escalate.
		{
			name:           "decompose/attempt_1",
			failureClass:   models.FailureClassDecompose,
			attempt:        1,
			wantAction:     CorrectionActionRetry,
			wantScope:      CorrectionScopeTemporary,
			wantHasBackoff: true,
		},
		{
			name:       "decompose/attempt_3_escalates",
			failureClass: models.FailureClassDecompose,
			attempt:    3,
			wantAction: CorrectionActionEscalate,
			wantScope:  CorrectionScopePermanent,
		},

		// Shutdown: retry immediately.
		{
			name:       "shutdown/attempt_1",
			failureClass: models.FailureClassShutdown,
			attempt:    1,
			wantAction: CorrectionActionRetry,
			wantScope:  CorrectionScopeTemporary,
		},

		// Unknown: retry with context, then escalate.
		{
			name:           "unknown/attempt_1",
			failureClass:   models.FailureClassUnknown,
			attempt:        1,
			wantAction:     CorrectionActionRetryWithContext,
			wantScope:      CorrectionScopeTemporary,
			wantHasBackoff: true,
		},
		{
			name:       "unknown/attempt_3_escalates",
			failureClass: models.FailureClassUnknown,
			attempt:    3,
			wantAction: CorrectionActionEscalate,
			wantScope:  CorrectionScopePermanent,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			decision := decideCorrection(tc.failureClass, issueNum, tc.attempt, timeout)

			if decision.Action != tc.wantAction {
				t.Errorf("action: got %q, want %q", decision.Action, tc.wantAction)
			}
			if decision.Scope != tc.wantScope {
				t.Errorf("scope: got %q, want %q", decision.Scope, tc.wantScope)
			}
			if decision.IssueNumber != issueNum {
				t.Errorf("issue_number: got %d, want %d", decision.IssueNumber, issueNum)
			}
			if decision.FailureClass != tc.failureClass {
				t.Errorf("failure_class: got %q, want %q", decision.FailureClass, tc.failureClass)
			}
			if decision.Attempt != tc.attempt {
				t.Errorf("attempt: got %d, want %d", decision.Attempt, tc.attempt)
			}
			if tc.wantHasBackoff && decision.BackoffUntil.IsZero() {
				t.Error("expected non-zero BackoffUntil")
			}
			if !tc.wantHasBackoff && !decision.BackoffUntil.IsZero() {
				t.Error("expected zero BackoffUntil")
			}
			if decision.Reason == "" {
				t.Error("expected non-empty reason")
			}
		})
	}
}

func TestDecideCorrection_TimeoutIncreasesTimeout(t *testing.T) {
	timeout := 5 * time.Minute
	decision := decideCorrection(models.FailureClassTimeout, 1, 1, timeout)

	wantTimeout := time.Duration(float64(timeout) * 1.5)
	if decision.NewTimeout != wantTimeout {
		t.Errorf("new_timeout: got %v, want %v", decision.NewTimeout, wantTimeout)
	}
}

func TestBackoffWithJitter(t *testing.T) {
	before := time.Now()
	result := backoffWithJitter(1)

	// Attempt 1 should be ~1min in the future (with +-25% jitter).
	minExpected := before.Add(45 * time.Second)
	maxExpected := before.Add(75 * time.Second)

	if result.Before(minExpected) || result.After(maxExpected) {
		t.Errorf("backoff attempt 1: got %v, want between %v and %v", result, minExpected, maxExpected)
	}

	// Attempt 2 should be ~2min.
	before2 := time.Now()
	result2 := backoffWithJitter(2)
	minExpected2 := before2.Add(90 * time.Second)
	maxExpected2 := before2.Add(150 * time.Second)

	if result2.Before(minExpected2) || result2.After(maxExpected2) {
		t.Errorf("backoff attempt 2: got %v, want between %v and %v", result2, minExpected2, maxExpected2)
	}
}

func TestBackoffWithJitter_ZeroAttempt(t *testing.T) {
	// attempt < 1 should be clamped to 1.
	before := time.Now()
	result := backoffWithJitter(0)
	if result.Before(before.Add(45 * time.Second)) {
		t.Error("attempt 0 should be clamped to attempt 1")
	}
}

func TestAllFailureClassesCovered(t *testing.T) {
	// Verify that every failure class produces a non-empty action.
	allClasses := []models.FailureClass{
		models.FailureClassAuth,
		models.FailureClassBudget,
		models.FailureClassPermanent,
		models.FailureClassTimeout,
		models.FailureClassProviderDown,
		models.FailureClassOOMKill,
		models.FailureClassShutdown,
		models.FailureClassPanic,
		models.FailureClassPostProcess,
		models.FailureClassClassify,
		models.FailureClassCycle,
		models.FailureClassDecompose,
		models.FailureClassUnknown,
	}

	for _, fc := range allClasses {
		t.Run(string(fc), func(t *testing.T) {
			decision := decideCorrection(fc, 1, 1, 5*time.Minute)
			if decision.Action == "" {
				t.Errorf("failure class %q produced empty action", fc)
			}
			if decision.Scope == "" {
				t.Errorf("failure class %q produced empty scope", fc)
			}
			if decision.Reason == "" {
				t.Errorf("failure class %q produced empty reason", fc)
			}
		})
	}
}
