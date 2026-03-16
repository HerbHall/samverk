package dispatcher

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/herbhall/samverk/internal/forge"
	"github.com/herbhall/samverk/pkg/models"
)

// mockProjectResolver implements ProjectResolver for testing.
type mockProjectResolver struct {
	trackers map[string]forge.IssueTracker // key: "owner/repo"
}

func newMockProjectResolver() *mockProjectResolver {
	return &mockProjectResolver{
		trackers: make(map[string]forge.IssueTracker),
	}
}

func (r *mockProjectResolver) addProject(owner, repo string, tracker forge.IssueTracker) {
	r.trackers[owner+"/"+repo] = tracker
}

func (r *mockProjectResolver) TrackerFor(owner, repo string) (forge.IssueTracker, bool) {
	t, ok := r.trackers[owner+"/"+repo]
	return t, ok
}

// issueBodyMixed builds a frontmatter body with mixed local and cross-project deps.
func issueBodyMixed(agentType string, localDeps []int, crossDeps []string) string {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("schema_version: \"1.1.0\"\n")
	b.WriteString("type: task\n")
	if agentType != "" {
		fmt.Fprintf(&b, "agent_type: %s\n", agentType)
	}
	b.WriteString("priority: normal\n")
	if len(localDeps) > 0 || len(crossDeps) > 0 {
		b.WriteString("depends_on:\n")
		for _, dep := range localDeps {
			fmt.Fprintf(&b, "  - %d\n", dep)
		}
		for _, dep := range crossDeps {
			fmt.Fprintf(&b, "  - \"%s\"\n", dep)
		}
	}
	b.WriteString("---\n\n## Summary\n\nTest issue.\n")
	return b.String()
}

func TestCheckDependencies_SameProjectOnly(t *testing.T) {
	tracker := newMockTracker()
	d := newTestDispatcher(tracker)

	closedAt := time.Now()
	tracker.issues[10] = &forge.Issue{
		Number:   10,
		State:    forge.StateClosed,
		Labels:   []string{"status:done"},
		ClosedAt: &closedAt,
	}
	tracker.issues[11] = &forge.Issue{
		Number: 11,
		State:  forge.StateOpen,
		Labels: []string{"status:in-progress"},
	}

	fm := &models.IssueFrontmatter{
		DependsOn: models.DependencyList{
			{Number: 10},
			{Number: 11},
		},
	}

	blocked, blockers, err := d.checkDependencies(context.Background(), "test", "repo", fm)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !blocked {
		t.Error("expected blocked=true")
	}
	if len(blockers) != 1 || blockers[0] != "11" {
		t.Errorf("blockers = %v, want [11]", blockers)
	}
}

func TestCheckDependencies_CrossProjectDone(t *testing.T) {
	tracker := newMockTracker()
	d := newTestDispatcher(tracker)

	// Set up cross-project tracker with a done issue.
	crossTracker := newMockTracker()
	closedAt := time.Now()
	crossTracker.issues[15] = &forge.Issue{
		Number:   15,
		State:    forge.StateClosed,
		Labels:   []string{"status:done"},
		ClosedAt: &closedAt,
	}

	resolver := newMockProjectResolver()
	resolver.addProject("HerbHall", "devkit", crossTracker)
	d.projects = resolver

	fm := &models.IssueFrontmatter{
		DependsOn: models.DependencyList{
			{Owner: "HerbHall", Repo: "devkit", Number: 15},
		},
	}

	blocked, blockers, err := d.checkDependencies(context.Background(), "test", "repo", fm)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if blocked {
		t.Error("expected blocked=false (cross-project dep is done)")
	}
	if len(blockers) != 0 {
		t.Errorf("blockers = %v, want empty", blockers)
	}
}

func TestCheckDependencies_CrossProjectNotDone(t *testing.T) {
	tracker := newMockTracker()
	d := newTestDispatcher(tracker)

	crossTracker := newMockTracker()
	crossTracker.issues[15] = &forge.Issue{
		Number: 15,
		State:  forge.StateOpen,
		Labels: []string{"status:in-progress"},
	}

	resolver := newMockProjectResolver()
	resolver.addProject("HerbHall", "devkit", crossTracker)
	d.projects = resolver

	fm := &models.IssueFrontmatter{
		DependsOn: models.DependencyList{
			{Owner: "HerbHall", Repo: "devkit", Number: 15},
		},
	}

	blocked, blockers, err := d.checkDependencies(context.Background(), "test", "repo", fm)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !blocked {
		t.Error("expected blocked=true")
	}
	if len(blockers) != 1 || blockers[0] != "HerbHall/devkit#15" {
		t.Errorf("blockers = %v, want [HerbHall/devkit#15]", blockers)
	}
}

func TestCheckDependencies_MixedDeps(t *testing.T) {
	tracker := newMockTracker()
	d := newTestDispatcher(tracker)

	// Local dep is done.
	closedAt := time.Now()
	tracker.issues[42] = &forge.Issue{
		Number:   42,
		State:    forge.StateClosed,
		Labels:   []string{"status:done"},
		ClosedAt: &closedAt,
	}

	// Cross-project dep is NOT done.
	crossTracker := newMockTracker()
	crossTracker.issues[15] = &forge.Issue{
		Number: 15,
		State:  forge.StateOpen,
		Labels: []string{"status:queued"},
	}

	resolver := newMockProjectResolver()
	resolver.addProject("HerbHall", "devkit", crossTracker)
	d.projects = resolver

	fm := &models.IssueFrontmatter{
		DependsOn: models.DependencyList{
			{Number: 42},
			{Owner: "HerbHall", Repo: "devkit", Number: 15},
		},
	}

	blocked, blockers, err := d.checkDependencies(context.Background(), "test", "repo", fm)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !blocked {
		t.Error("expected blocked=true (cross-project dep not done)")
	}
	if len(blockers) != 1 || blockers[0] != "HerbHall/devkit#15" {
		t.Errorf("blockers = %v, want [HerbHall/devkit#15]", blockers)
	}
}

func TestCheckDependencies_CrossProjectUnregistered(t *testing.T) {
	tracker := newMockTracker()
	d := newTestDispatcher(tracker)

	resolver := newMockProjectResolver()
	d.projects = resolver

	fm := &models.IssueFrontmatter{
		DependsOn: models.DependencyList{
			{Owner: "HerbHall", Repo: "nonexistent", Number: 1},
		},
	}

	_, _, err := d.checkDependencies(context.Background(), "test", "repo", fm)
	if err == nil {
		t.Fatal("expected error for unregistered project")
	}
	if !strings.Contains(err.Error(), "not registered") {
		t.Errorf("error = %q, want to contain 'not registered'", err.Error())
	}
}

func TestCheckDependencies_NoProjectResolver(t *testing.T) {
	tracker := newMockTracker()
	d := newTestDispatcher(tracker)
	// d.projects is nil

	fm := &models.IssueFrontmatter{
		DependsOn: models.DependencyList{
			{Owner: "HerbHall", Repo: "devkit", Number: 15},
		},
	}

	_, _, err := d.checkDependencies(context.Background(), "test", "repo", fm)
	if err == nil {
		t.Fatal("expected error when no project resolver is configured")
	}
	if !strings.Contains(err.Error(), "no project resolver") {
		t.Errorf("error = %q, want to contain 'no project resolver'", err.Error())
	}
}

func TestCheckDependencies_EmptyDeps(t *testing.T) {
	tracker := newMockTracker()
	d := newTestDispatcher(tracker)

	fm := &models.IssueFrontmatter{
		DependsOn: nil,
	}

	blocked, blockers, err := d.checkDependencies(context.Background(), "test", "repo", fm)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if blocked {
		t.Error("expected blocked=false for empty deps")
	}
	if len(blockers) != 0 {
		t.Errorf("blockers = %v, want empty", blockers)
	}
}

func TestCheckDependencies_NilFrontmatter(t *testing.T) {
	tracker := newMockTracker()
	d := newTestDispatcher(tracker)

	blocked, blockers, err := d.checkDependencies(context.Background(), "test", "repo", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if blocked {
		t.Error("expected blocked=false for nil frontmatter")
	}
	if len(blockers) != 0 {
		t.Errorf("blockers = %v, want empty", blockers)
	}
}

func TestRecheckCrossProjectDeps_UnblocksWhenDone(t *testing.T) {
	tracker := newMockTracker()
	d := newTestDispatcher(tracker)

	// Local dep is done.
	closedAt := time.Now()
	tracker.issues[42] = &forge.Issue{
		Number:   42,
		State:    forge.StateClosed,
		Labels:   []string{"status:done"},
		ClosedAt: &closedAt,
	}

	// Blocked issue with both local and cross-project deps.
	tracker.issues[1] = &forge.Issue{
		Number: 1,
		Title:  "Blocked on cross-project dep",
		Body:   issueBodyMixed("code-gen", []int{42}, []string{"HerbHall/devkit#15"}),
		State:  forge.StateOpen,
		Labels: []string{"status:blocked"},
	}

	// Cross-project dep is now done.
	crossTracker := newMockTracker()
	crossTracker.issues[15] = &forge.Issue{
		Number:   15,
		State:    forge.StateClosed,
		Labels:   []string{"status:done"},
		ClosedAt: &closedAt,
	}

	resolver := newMockProjectResolver()
	resolver.addProject("HerbHall", "devkit", crossTracker)
	d.projects = resolver

	err := d.recheckCrossProjectDeps(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	issue := tracker.issues[1]
	if hasLabel(issue.Labels, "status:blocked") {
		t.Error("expected status:blocked to be removed")
	}
	if !hasLabel(issue.Labels, "status:queued") {
		t.Error("expected status:queued after cross-project unblock")
	}
}

func TestRecheckCrossProjectDeps_StaysBlockedWhenNotDone(t *testing.T) {
	tracker := newMockTracker()
	d := newTestDispatcher(tracker)

	tracker.issues[1] = &forge.Issue{
		Number: 1,
		Title:  "Blocked on cross-project dep",
		Body:   issueBodyMixed("code-gen", nil, []string{"HerbHall/devkit#15"}),
		State:  forge.StateOpen,
		Labels: []string{"status:blocked"},
	}

	// Cross-project dep is still open.
	crossTracker := newMockTracker()
	crossTracker.issues[15] = &forge.Issue{
		Number: 15,
		State:  forge.StateOpen,
		Labels: []string{"status:in-progress"},
	}

	resolver := newMockProjectResolver()
	resolver.addProject("HerbHall", "devkit", crossTracker)
	d.projects = resolver

	err := d.recheckCrossProjectDeps(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	issue := tracker.issues[1]
	if !hasLabel(issue.Labels, "status:blocked") {
		t.Error("expected status:blocked to remain")
	}
	if hasLabel(issue.Labels, "status:queued") {
		t.Error("did not expect status:queued (dep not done)")
	}
}

func TestRecheckCrossProjectDeps_SkipsWithoutResolver(t *testing.T) {
	tracker := newMockTracker()
	d := newTestDispatcher(tracker)
	// d.projects is nil

	err := d.recheckCrossProjectDeps(context.Background())
	if err != nil {
		t.Fatalf("expected nil error when no resolver, got: %v", err)
	}
}

func TestRecheckCrossProjectDeps_SkipsLocalOnlyDeps(t *testing.T) {
	tracker := newMockTracker()
	d := newTestDispatcher(tracker)

	// Blocked issue with only local deps -- should not be processed.
	tracker.issues[1] = &forge.Issue{
		Number: 1,
		Title:  "Blocked on local dep only",
		Body:   issueBody("code-gen", []int{42}),
		State:  forge.StateOpen,
		Labels: []string{"status:blocked"},
	}
	tracker.issues[42] = &forge.Issue{
		Number: 42,
		State:  forge.StateOpen,
		Labels: []string{"status:in-progress"},
	}

	resolver := newMockProjectResolver()
	d.projects = resolver

	err := d.recheckCrossProjectDeps(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Issue should remain blocked (local-only deps are not touched by this function).
	issue := tracker.issues[1]
	if !hasLabel(issue.Labels, "status:blocked") {
		t.Error("expected status:blocked to remain for local-only dep issue")
	}
}

func TestBuildDependencyGraph_ExcludesCrossProject(t *testing.T) {
	tracker := newMockTracker()
	d := newTestDispatcher(tracker)

	// Issue with mixed deps.
	tracker.issues[1] = &forge.Issue{
		Number: 1,
		State:  forge.StateOpen,
		Body:   issueBodyMixed("code-gen", []int{2, 3}, []string{"HerbHall/devkit#15"}),
	}
	tracker.issues[2] = &forge.Issue{
		Number: 2,
		State:  forge.StateOpen,
		Body:   issueBody("code-gen", nil),
	}
	tracker.issues[3] = &forge.Issue{
		Number: 3,
		State:  forge.StateOpen,
		Body:   issueBody("code-gen", nil),
	}

	graph, err := d.buildDependencyGraph(context.Background(), "test", "repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	deps, ok := graph[1]
	if !ok {
		t.Fatal("expected issue #1 in graph")
	}

	// Should only contain local deps (2, 3), not cross-project.
	if len(deps) != 2 {
		t.Errorf("deps length = %d, want 2", len(deps))
	}
	for _, dep := range deps {
		if dep != 2 && dep != 3 {
			t.Errorf("unexpected dep %d in graph", dep)
		}
	}
}

func TestHandleOpened_CrossProjectDependencyBlocked(t *testing.T) {
	tracker := newMockTracker()
	d := newTestDispatcher(tracker)

	// Cross-project dep is NOT done.
	crossTracker := newMockTracker()
	crossTracker.issues[15] = &forge.Issue{
		Number: 15,
		State:  forge.StateOpen,
		Labels: []string{"status:queued"},
	}

	resolver := newMockProjectResolver()
	resolver.addProject("HerbHall", "devkit", crossTracker)
	d.projects = resolver

	issue := &forge.Issue{
		Number: 1,
		Title:  "Depends on cross-project",
		Body:   issueBodyMixed("code-gen", nil, []string{"HerbHall/devkit#15"}),
		State:  forge.StateOpen,
		Labels: []string{"status:queued"},
	}
	tracker.issues[1] = issue

	err := d.handleOpened(context.Background(), forge.Event{
		Type:        forge.EventIssueOpened,
		IssueNumber: 1,
		Issue:       issue,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !hasLabel(issue.Labels, "status:blocked") {
		t.Error("expected status:blocked for unsatisfied cross-project dependency")
	}
}

func TestHandleOpened_CrossProjectDependencyDone(t *testing.T) {
	tracker := newMockTracker()
	d := newTestDispatcher(tracker)

	// Cross-project dep IS done.
	crossTracker := newMockTracker()
	closedAt := time.Now()
	crossTracker.issues[15] = &forge.Issue{
		Number:   15,
		State:    forge.StateClosed,
		Labels:   []string{"status:done"},
		ClosedAt: &closedAt,
	}

	resolver := newMockProjectResolver()
	resolver.addProject("HerbHall", "devkit", crossTracker)
	d.projects = resolver

	issue := &forge.Issue{
		Number: 1,
		Title:  "Depends on cross-project (done)",
		Body:   issueBodyMixed("code-gen", nil, []string{"HerbHall/devkit#15"}),
		State:  forge.StateOpen,
		Labels: []string{"status:queued"},
	}
	tracker.issues[1] = issue

	err := d.handleOpened(context.Background(), forge.Event{
		Type:        forge.EventIssueOpened,
		IssueNumber: 1,
		Issue:       issue,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if hasLabel(issue.Labels, "status:blocked") {
		t.Error("did not expect status:blocked -- cross-project dep is done")
	}
	if !hasLabel(issue.Labels, "status:claimed") {
		t.Error("expected status:claimed after routing")
	}
}

func TestSetProjectResolver(t *testing.T) {
	tracker := newMockTracker()
	d := newTestDispatcher(tracker)

	if d.projects != nil {
		t.Error("expected nil projects initially")
	}

	resolver := newMockProjectResolver()
	d.SetProjectResolver(resolver)

	if d.projects == nil {
		t.Error("expected non-nil projects after SetProjectResolver")
	}
}

func TestDetectCycle_MixedDeps_IgnoresCrossProject(t *testing.T) {
	tracker := newMockTracker()
	d := newTestDispatcher(tracker)
	logger := zap.NewNop()
	d.logger = logger

	// Linear chain with cross-project dep: 1 depends on 2 and cross-project.
	// 2 depends on 3. No cycle.
	tracker.issues[1] = &forge.Issue{
		Number: 1,
		State:  forge.StateOpen,
		Body:   issueBodyMixed("code-gen", []int{2}, []string{"HerbHall/devkit#99"}),
	}
	tracker.issues[2] = &forge.Issue{
		Number: 2,
		State:  forge.StateOpen,
		Body:   issueBody("code-gen", []int{3}),
	}
	tracker.issues[3] = &forge.Issue{
		Number: 3,
		State:  forge.StateOpen,
		Body:   issueBody("code-gen", nil),
	}

	cycle, err := d.detectCycle(context.Background(), "test", "repo", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cycle != nil {
		t.Errorf("expected no cycle, got %v", cycle)
	}
}
