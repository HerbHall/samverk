package digest

import (
	"strings"
	"testing"
	"time"
)

func TestRenderPRSection_WithPRs(t *testing.T) {
	d := DigestData{
		LastCheckIn: time.Now().Add(-24 * time.Hour),
		QueuedCount: 1,
		PRsAwaiting: []PRAwaitingReview{
			{Project: "samverk", Number: 10, Title: "feat: add widget", Author: "bot", Tier: "tier-2", CIStatus: "passed", Age: "2h"},
			{Project: "samverk", Number: 11, Title: "docs: update readme", Author: "agent", Tier: "tier-1", CIStatus: "passed", Age: "30m"},
		},
		AutoMergedCount: 3,
	}

	out := FormatDigest(d)

	if !strings.Contains(out, "PRS AWAITING REVIEW") {
		t.Error("expected PRS AWAITING REVIEW section")
	}
	if !strings.Contains(out, "Auto-merged since last check-in: 3 PRs") {
		t.Errorf("expected auto-merged count, got:\n%s", out)
	}
	if !strings.Contains(out, "#10 feat: add widget") {
		t.Error("expected PR #10 in output")
	}
	if !strings.Contains(out, "[tier-2, CI: passed, 2h]") {
		t.Error("expected tier and CI info for PR #10")
	}
	if !strings.Contains(out, "#11 docs: update readme") {
		t.Error("expected PR #11 in output")
	}
}

func TestRenderPRSection_NoPRs(t *testing.T) {
	d := DigestData{
		LastCheckIn:     time.Now().Add(-24 * time.Hour),
		QueuedCount:     1,
		AutoMergedCount: 5,
	}

	out := FormatDigest(d)

	if !strings.Contains(out, "PRS AWAITING REVIEW") {
		t.Error("expected PRS AWAITING REVIEW section even with no PRs")
	}
	if !strings.Contains(out, "Auto-merged since last check-in: 5 PRs") {
		t.Error("expected auto-merged count")
	}
	if !strings.Contains(out, "No open PRs awaiting review") {
		t.Error("expected 'No open PRs' message")
	}
}

func TestRenderPRSection_MultiProject(t *testing.T) {
	d := DigestData{
		LastCheckIn: time.Now().Add(-24 * time.Hour),
		QueuedCount: 1,
		PRsAwaiting: []PRAwaitingReview{
			{Project: "alpha", Number: 1, Title: "PR A", Author: "bot", Tier: "tier-1", CIStatus: "passed", Age: "1h"},
			{Project: "beta", Number: 2, Title: "PR B", Author: "bot", Tier: "tier-2", CIStatus: "pending", Age: "3h"},
		},
	}

	out := FormatDigest(d)

	if !strings.Contains(out, "alpha:") {
		t.Error("expected project name 'alpha' in grouped output")
	}
	if !strings.Contains(out, "beta:") {
		t.Error("expected project name 'beta' in grouped output")
	}
}

func TestRenderPRSection_Omitted_WhenNoPRsAndNoAutoMerged(t *testing.T) {
	d := DigestData{
		LastCheckIn: time.Now().Add(-24 * time.Hour),
		QueuedCount: 1,
	}

	out := FormatDigest(d)

	if strings.Contains(out, "PRS AWAITING REVIEW") {
		t.Error("PR section should be omitted when no PRs and no auto-merged count")
	}
}
