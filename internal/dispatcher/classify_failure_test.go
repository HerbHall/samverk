package dispatcher

import (
	"testing"

	"github.com/herbhall/samverk/pkg/models"
)

func TestClassifyFailure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  models.FailureClass
	}{
		// --- empty / unknown ---
		{
			name:  "empty string returns unknown",
			input: "",
			want:  models.FailureClassUnknown,
		},
		{
			name:  "unrecognised message returns unknown",
			input: "something went wrong",
			want:  models.FailureClassUnknown,
		},
		{
			name:  "whitespace-only returns unknown",
			input: "   ",
			want:  models.FailureClassUnknown,
		},

		// --- shutdown ---
		{
			name:  "exit status 143 is shutdown",
			input: "exit status 143",
			want:  models.FailureClassShutdown,
		},
		{
			name:  "signal: terminated is shutdown",
			input: "signal: terminated",
			want:  models.FailureClassShutdown,
		},
		{
			name:  "shutdown: case-insensitive EXIT STATUS 143",
			input: "EXIT STATUS 143",
			want:  models.FailureClassShutdown,
		},
		{
			name:  "shutdown: case-insensitive SIGNAL: TERMINATED",
			input: "SIGNAL: TERMINATED",
			want:  models.FailureClassShutdown,
		},
		{
			name:  "shutdown wins over timeout keyword (exit status 143 has no timeout in it)",
			input: "process ended: exit status 143",
			want:  models.FailureClassShutdown,
		},

		// --- panic ---
		{
			name:  "send on closed channel is panic",
			input: "send on closed channel",
			want:  models.FailureClassPanic,
		},
		{
			name:  "runtime error is panic",
			input: "runtime error: index out of range [0] with length 0",
			want:  models.FailureClassPanic,
		},
		{
			name:  "panic: prefix is panic",
			input: "panic: assignment to entry in nil map",
			want:  models.FailureClassPanic,
		},
		{
			name:  "panic: case-insensitive PANIC:",
			input: "PANIC: something fatal",
			want:  models.FailureClassPanic,
		},
		{
			name:  "panic: case-insensitive RUNTIME ERROR",
			input: "RUNTIME ERROR: nil dereference",
			want:  models.FailureClassPanic,
		},

		// --- shutdown beats panic (shutdown checked first in source) ---
		// "exit status 143" doesn't contain any panic keyword, so no conflict.
		// "signal: terminated" likewise. No overlap to test here, so we verify
		// that shutdown patterns cannot accidentally reach the panic branch.
		{
			name:  "signal: terminated does not become panic",
			input: "signal: terminated and something weird",
			want:  models.FailureClassShutdown,
		},

		// --- auth ---
		{
			name:  "401 is auth",
			input: "HTTP 401",
			want:  models.FailureClassAuth,
		},
		{
			name:  "authentication_error is auth",
			input: "authentication_error from provider",
			want:  models.FailureClassAuth,
		},
		{
			name:  "oauth token expired is auth",
			input: "oauth token expired, please refresh",
			want:  models.FailureClassAuth,
		},
		{
			name:  "unauthorized is auth",
			input: "unauthorized request",
			want:  models.FailureClassAuth,
		},
		{
			name:  "invalid api key is auth",
			input: "invalid api key provided",
			want:  models.FailureClassAuth,
		},
		{
			name:  "invalid x-api-key is auth",
			input: "invalid x-api-key header",
			want:  models.FailureClassAuth,
		},
		{
			name:  "auth: case-insensitive UNAUTHORIZED",
			input: "UNAUTHORIZED",
			want:  models.FailureClassAuth,
		},
		{
			name:  "auth: 401 embedded in longer message",
			input: "provider returned status 401 with body: ...",
			want:  models.FailureClassAuth,
		},
		{
			name:  "not logged in is auth",
			input: "claude-cli: exec: exit status 1: output: Not logged in · Please run /login",
			want:  models.FailureClassAuth,
		},

		// --- budget ---
		{
			name:  "credit balance is too low is budget",
			input: "credit balance is too low for this request",
			want:  models.FailureClassBudget,
		},
		{
			name:  "budget exceeded is budget",
			input: "budget exceeded for project samverk",
			want:  models.FailureClassBudget,
		},
		{
			name:  "insufficient_quota is budget",
			input: "insufficient_quota on model claude-3-5-sonnet",
			want:  models.FailureClassBudget,
		},
		{
			name:  "rate_limit is budget",
			input: "rate_limit hit, slow down",
			want:  models.FailureClassBudget,
		},
		{
			name:  "budget: case-insensitive RATE_LIMIT",
			input: "RATE_LIMIT exceeded",
			want:  models.FailureClassBudget,
		},
		{
			name:  "budget: case-insensitive BUDGET EXCEEDED",
			input: "BUDGET EXCEEDED",
			want:  models.FailureClassBudget,
		},

		// --- permanent ---
		{
			name:  "model not found is permanent",
			input: "model not found",
			want:  models.FailureClassPermanent,
		},
		{
			name:  "model name with not found is permanent",
			input: "model claude-3-opus-20240229 not found in registry",
			want:  models.FailureClassPermanent,
		},
		{
			name:  "permanent: case-insensitive MODEL NOT FOUND",
			input: "MODEL NOT FOUND",
			want:  models.FailureClassPermanent,
		},
		// "model" without "not found" must NOT become permanent.
		{
			name:  "model alone without not found is unknown",
			input: "loading model weights",
			want:  models.FailureClassUnknown,
		},
		// "not found" without "model" must NOT become permanent.
		{
			name:  "not found without model is unknown",
			input: "resource not found in store",
			want:  models.FailureClassUnknown,
		},

		// --- oom_kill ---
		{
			name:  "signal: killed is oom_kill",
			input: "signal: killed",
			want:  models.FailureClassOOMKill,
		},
		{
			name:  "oom_kill: case-insensitive SIGNAL: KILLED",
			input: "SIGNAL: KILLED",
			want:  models.FailureClassOOMKill,
		},
		{
			name:  "oom_kill: embedded in process message",
			input: "agent process exited: signal: killed",
			want:  models.FailureClassOOMKill,
		},
		// "signal: killed" must NOT become shutdown (which uses "signal: terminated").
		{
			name:  "signal: killed does not become shutdown",
			input: "signal: killed by kernel",
			want:  models.FailureClassOOMKill,
		},

		// --- provider_down ---
		{
			name:  "context deadline exceeded is provider_down",
			input: "context deadline exceeded",
			want:  models.FailureClassProviderDown,
		},
		{
			name:  "connection refused is provider_down",
			input: "dial tcp: connection refused",
			want:  models.FailureClassProviderDown,
		},
		{
			name:  "no such host is provider_down",
			input: "dial tcp: no such host",
			want:  models.FailureClassProviderDown,
		},
		{
			name:  "i/o timeout is provider_down",
			input: "read tcp: i/o timeout",
			want:  models.FailureClassProviderDown,
		},
		{
			name:  "no healthy provider is provider_down",
			input: "no healthy provider available in registry",
			want:  models.FailureClassProviderDown,
		},
		{
			name:  "provider_down: case-insensitive CONTEXT DEADLINE EXCEEDED",
			input: "CONTEXT DEADLINE EXCEEDED",
			want:  models.FailureClassProviderDown,
		},
		{
			name:  "provider_down: mixed-case message",
			input: "provider error: Context Deadline Exceeded waiting for response",
			want:  models.FailureClassProviderDown,
		},

		// --- timeout ---
		{
			name:  "timeout keyword is timeout",
			input: "agent session timeout after 30 minutes",
			want:  models.FailureClassTimeout,
		},
		{
			name:  "missed heartbeat is timeout",
			input: "missed heartbeat from worker",
			want:  models.FailureClassTimeout,
		},
		{
			name:  "timeout: case-insensitive TIMEOUT",
			input: "TIMEOUT waiting for agent",
			want:  models.FailureClassTimeout,
		},
		{
			name:  "timeout: case-insensitive MISSED HEARTBEAT",
			input: "MISSED HEARTBEAT after 120s",
			want:  models.FailureClassTimeout,
		},
		// provider_down is checked before timeout; "context deadline exceeded"
		// must NOT fall through to timeout even though it is a timing failure.
		{
			name:  "context deadline exceeded does not become timeout",
			input: "context deadline exceeded in transport",
			want:  models.FailureClassProviderDown,
		},
		// "i/o timeout" contains "timeout" but provider_down is checked first.
		{
			name:  "i/o timeout does not become generic timeout",
			input: "i/o timeout on socket read",
			want:  models.FailureClassProviderDown,
		},

		// --- post_process ---
		{
			name:  "post-process error is post_process",
			input: "post-process error: could not update labels",
			want:  models.FailureClassPostProcess,
		},
		{
			name:  "create pr: is post_process",
			input: "create pr: branch already exists",
			want:  models.FailureClassPostProcess,
		},
		{
			name:  "create branch is post_process",
			input: "create branch feature/issue-42 failed",
			want:  models.FailureClassPostProcess,
		},
		{
			name:  "add comment: is post_process",
			input: "add comment: rate limited by GitHub",
			want:  models.FailureClassPostProcess,
		},
		{
			name:  "post_process: case-insensitive POST-PROCESS ERROR",
			input: "POST-PROCESS ERROR: label update failed",
			want:  models.FailureClassPostProcess,
		},
		{
			name:  "post_process: case-insensitive CREATE PR:",
			input: "CREATE PR: cannot create pull request",
			want:  models.FailureClassPostProcess,
		},
		{
			name:  "validation failed is post_process",
			input: "validation failed (not retryable): go build failed",
			want:  models.FailureClassPostProcess,
		},
		{
			name:  "validation retry failed is post_process",
			input: "validation retry failed: validation failed after retry: go test failed",
			want:  models.FailureClassPostProcess,
		},

		// --- classify ---
		{
			name:  "classify issue is classify",
			input: "classify issue #42: unexpected format",
			want:  models.FailureClassClassify,
		},
		{
			name:  "invalid_frontmatter is classify",
			input: "invalid_frontmatter detected in issue body",
			want:  models.FailureClassClassify,
		},
		{
			name:  "no frontmatter found is classify",
			input: "no frontmatter found in issue #7",
			want:  models.FailureClassClassify,
		},
		{
			name:  "unknown agent_type is classify",
			input: "unknown agent_type: agent:wizard",
			want:  models.FailureClassClassify,
		},
		{
			name:  "classify: case-insensitive NO FRONTMATTER FOUND",
			input: "NO FRONTMATTER FOUND",
			want:  models.FailureClassClassify,
		},

		// --- cycle ---
		{
			name:  "dependency_cycle is cycle",
			input: "dependency_cycle involving issues #1 #2 #3",
			want:  models.FailureClassCycle,
		},
		{
			name:  "cycle detected is cycle",
			input: "cycle detected in dependency graph",
			want:  models.FailureClassCycle,
		},
		{
			name:  "cycle: case-insensitive DEPENDENCY_CYCLE",
			input: "DEPENDENCY_CYCLE found",
			want:  models.FailureClassCycle,
		},

		// --- decompose ---
		{
			name:  "decompose keyword is decompose",
			input: "decompose failed: child issue creation error",
			want:  models.FailureClassDecompose,
		},
		{
			name:  "decompose: case-insensitive DECOMPOSE",
			input: "DECOMPOSE error for issue #88",
			want:  models.FailureClassDecompose,
		},

		// --- priority ordering: shutdown checked before panic ---
		// "exit status 143" contains no panic patterns, but verify the ordering
		// still holds when a message contains "runtime" near shutdown keywords.
		{
			name:  "signal: terminated wins over unrelated text containing runtime",
			input: "signal: terminated (runtime restart)",
			want:  models.FailureClassShutdown,
		},

		// --- priority ordering: panic checked before auth ---
		// A message with both "runtime error" and "401" should be panic
		// because panic is checked first.
		{
			name:  "panic wins over auth when runtime error and 401 co-occur",
			input: "runtime error: goroutine crashed, last response was 401",
			want:  models.FailureClassPanic,
		},

		// --- priority ordering: auth checked before budget ---
		// A message with "401" and "rate_limit" should be auth (auth checked first).
		{
			name:  "auth wins over budget when 401 and rate_limit co-occur",
			input: "received 401 after rate_limit window",
			want:  models.FailureClassAuth,
		},

		// --- priority ordering: budget checked before permanent ---
		// A message with "rate_limit" and "model not found" should be budget.
		{
			name:  "budget wins over permanent when rate_limit and model not found co-occur",
			input: "rate_limit exceeded for model not found tier",
			want:  models.FailureClassBudget,
		},

		// --- priority ordering: permanent checked before oom_kill ---
		// "model not found" must win over a stray "signal: killed" reference.
		{
			name:  "permanent wins over oom_kill when model not found and signal: killed co-occur",
			input: "model not found; previous attempt ended with signal: killed",
			want:  models.FailureClassPermanent,
		},

		// --- priority ordering: oom_kill checked before provider_down ---
		// A message with "signal: killed" and "connection refused" should be oom_kill.
		{
			name:  "oom_kill wins over provider_down when signal: killed and connection refused co-occur",
			input: "signal: killed — connection refused during cleanup",
			want:  models.FailureClassOOMKill,
		},

		// --- priority ordering: provider_down checked before timeout ---
		// Already covered by the "context deadline exceeded" and "i/o timeout" cases above.

		// --- priority ordering: timeout checked before post_process ---
		// A message with "timeout" and "create pr:" should be timeout.
		{
			name:  "timeout wins over post_process when both keywords present",
			input: "timeout waiting to create pr: push failed",
			want:  models.FailureClassTimeout,
		},

		// --- priority ordering: post_process checked before classify ---
		{
			name:  "post_process wins over classify when post-process error and classify issue co-occur",
			input: "post-process error: classify issue metadata rejected",
			want:  models.FailureClassPostProcess,
		},

		// --- priority ordering: classify checked before cycle ---
		{
			name:  "classify wins over cycle when classify issue and cycle detected co-occur",
			input: "classify issue: cycle detected in label chain",
			want:  models.FailureClassClassify,
		},

		// --- priority ordering: cycle checked before decompose ---
		{
			name:  "cycle wins over decompose when dependency_cycle and decompose co-occur",
			input: "dependency_cycle found while trying to decompose",
			want:  models.FailureClassCycle,
		},

		// --- real-world-style messages ---
		{
			name:  "real anthropic auth error",
			input: "error calling API: invalid x-api-key",
			want:  models.FailureClassAuth,
		},
		{
			name:  "real provider-down wrapped message",
			input: "provider error: context deadline exceeded (Client.Timeout exceeded while awaiting headers)",
			want:  models.FailureClassProviderDown,
		},
		{
			name:  "real DNS failure",
			input: "Get \"https://api.anthropic.com/v1/messages\": dial tcp: lookup api.anthropic.com: no such host",
			want:  models.FailureClassProviderDown,
		},
		{
			name:  "real budget exhaustion from OpenAI",
			input: "openai: insufficient_quota - you have exceeded your current quota",
			want:  models.FailureClassBudget,
		},
		{
			name:  "real missed heartbeat from agent monitor",
			input: "[monitor] issue #42 missed heartbeat threshold (3/3)",
			want:  models.FailureClassTimeout,
		},
		{
			name:  "real OOM from kernel log line",
			input: "agent subprocess for issue #99 exited: signal: killed",
			want:  models.FailureClassOOMKill,
		},
		{
			name:  "real panic stack trace prefix",
			input: "panic: runtime error: invalid memory address or nil pointer dereference",
			want:  models.FailureClassPanic,
		},
		{
			name:  "claude-cli hung with no output is provider_down",
			input: "provider chat: claude-cli: hung: no output for 3m0s: output: ",
			want:  models.FailureClassProviderDown,
		},
		{
			name:  "hung no output generic",
			input: "hung: no output for 5m0s",
			want:  models.FailureClassProviderDown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := classifyFailure(tt.input)
			if got != tt.want {
				t.Errorf("classifyFailure(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFailureClass_IsRetryable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		fc   models.FailureClass
		want bool
	}{
		// retryable
		{models.FailureClassTimeout, true},
		{models.FailureClassOOMKill, true},
		{models.FailureClassPostProcess, true},
		// not retryable — shutdown is expected, not a real failure
		{models.FailureClassShutdown, false},
		// not retryable — permanent failures
		{models.FailureClassAuth, false},
		{models.FailureClassBudget, false},
		{models.FailureClassPermanent, false},
		{models.FailureClassClassify, false},
		{models.FailureClassCycle, false},
		// not retryable — operational failures without a clear retry path
		{models.FailureClassProviderDown, false},
		{models.FailureClassPanic, false},
		{models.FailureClassDecompose, false},
		{models.FailureClassUnknown, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.fc), func(t *testing.T) {
			t.Parallel()
			got := tt.fc.IsRetryable()
			if got != tt.want {
				t.Errorf("FailureClass(%q).IsRetryable() = %v, want %v", tt.fc, got, tt.want)
			}
		})
	}
}

func TestFailureClass_IsPermanent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		fc   models.FailureClass
		want bool
	}{
		// permanent — retrying will never succeed
		{models.FailureClassAuth, true},
		{models.FailureClassBudget, true},
		{models.FailureClassPermanent, true},
		{models.FailureClassClassify, true},
		{models.FailureClassCycle, true},
		// not permanent — transient or recoverable
		{models.FailureClassTimeout, false},
		{models.FailureClassOOMKill, false},
		{models.FailureClassPostProcess, false},
		{models.FailureClassProviderDown, false},
		{models.FailureClassPanic, false},
		{models.FailureClassShutdown, false},
		{models.FailureClassDecompose, false},
		{models.FailureClassUnknown, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.fc), func(t *testing.T) {
			t.Parallel()
			got := tt.fc.IsPermanent()
			if got != tt.want {
				t.Errorf("FailureClass(%q).IsPermanent() = %v, want %v", tt.fc, got, tt.want)
			}
		})
	}
}

func TestFailureClass_IsRetryableAndIsPermanentAreMutuallyExclusive(t *testing.T) {
	t.Parallel()

	all := []models.FailureClass{
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

	for i := range all {
		fc := all[i]
		t.Run(string(fc), func(t *testing.T) {
			t.Parallel()
			if fc.IsRetryable() && fc.IsPermanent() {
				t.Errorf("FailureClass(%q) is both retryable and permanent — impossible state", fc)
			}
		})
	}
}

func TestFailureClass_IsProviderFailure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		fc   models.FailureClass
		want bool
	}{
		{models.FailureClassProviderDown, true},
		{models.FailureClassOOMKill, true},
		{models.FailureClassTimeout, false},
		{models.FailureClassAuth, false},
		{models.FailureClassBudget, false},
		{models.FailureClassPermanent, false},
		{models.FailureClassShutdown, false},
		{models.FailureClassPanic, false},
		{models.FailureClassPostProcess, false},
		{models.FailureClassClassify, false},
		{models.FailureClassCycle, false},
		{models.FailureClassDecompose, false},
		{models.FailureClassUnknown, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.fc), func(t *testing.T) {
			t.Parallel()
			got := tt.fc.IsProviderFailure()
			if got != tt.want {
				t.Errorf("FailureClass(%q).IsProviderFailure() = %v, want %v", tt.fc, got, tt.want)
			}
		})
	}
}

func BenchmarkClassifyFailure(b *testing.B) {
	inputs := []string{
		"",
		"context deadline exceeded",
		"model claude-3-opus not found",
		"runtime error: nil pointer dereference",
		"exit status 143",
		"invalid x-api-key",
		"rate_limit exceeded",
		"signal: killed",
		"missed heartbeat from worker after 120s",
		"something completely unrecognised that hits the fallback path",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		classifyFailure(inputs[i%len(inputs)])
	}
}
