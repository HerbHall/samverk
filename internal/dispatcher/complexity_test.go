package dispatcher

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/herbhall/samverk/internal/forge"
	"github.com/herbhall/samverk/internal/store"
	"github.com/herbhall/samverk/pkg/models"
	"go.uber.org/zap"
)

func TestEstimateTimeout_FrontmatterOverride(t *testing.T) {
	issue := &forge.Issue{Number: 1, Title: "test", Body: "short"}
	fm := &models.IssueFrontmatter{TimeoutMinutes: 25}

	got := EstimateTimeout(issue, fm, models.AgentTypeCodeGen, "default")
	if got != 25*time.Minute {
		t.Errorf("got %v, want 25m", got)
	}
}

func TestEstimateTimeout_FrontmatterClampMin(t *testing.T) {
	issue := &forge.Issue{Number: 1, Title: "test", Body: "short"}
	fm := &models.IssueFrontmatter{TimeoutMinutes: 1}

	got := EstimateTimeout(issue, fm, models.AgentTypeCodeGen, "default")
	if got != MinTimeout {
		t.Errorf("got %v, want %v", got, MinTimeout)
	}
}

func TestEstimateTimeout_FrontmatterClampMax(t *testing.T) {
	issue := &forge.Issue{Number: 1, Title: "test", Body: "short"}
	fm := &models.IssueFrontmatter{TimeoutMinutes: 120}

	got := EstimateTimeout(issue, fm, models.AgentTypeCodeGen, "default")
	if got != MaxTimeout {
		t.Errorf("got %v, want %v", got, MaxTimeout)
	}
}

func TestEstimateTimeout_NoFrontmatter(t *testing.T) {
	issue := &forge.Issue{Number: 1, Title: "test", Body: "short body"}

	got := EstimateTimeout(issue, nil, models.AgentTypeCodeGen, "default")
	if got < MinTimeout || got > MaxTimeout {
		t.Errorf("got %v, want between %v and %v", got, MinTimeout, MaxTimeout)
	}
}

func TestEstimateTimeout_LongBodyIncreasesTimeout(t *testing.T) {
	short := &forge.Issue{Number: 1, Title: "test", Body: "short"}
	long := &forge.Issue{Number: 2, Title: "test", Body: strings.Repeat("word ", 500)}

	shortTimeout := EstimateTimeout(short, nil, models.AgentTypeCodeGen, "default")
	longTimeout := EstimateTimeout(long, nil, models.AgentTypeCodeGen, "default")

	if longTimeout <= shortTimeout {
		t.Errorf("long body timeout %v should exceed short body timeout %v", longTimeout, shortTimeout)
	}
}

func TestEstimateTimeout_CheckboxesIncreaseTimeout(t *testing.T) {
	noChecks := &forge.Issue{Number: 1, Title: "test", Body: strings.Repeat("word ", 210)}
	withChecks := &forge.Issue{
		Number: 2,
		Title:  "test",
		Body:   strings.Repeat("word ", 210) + "\n- [ ] task one\n- [ ] task two\n- [x] task three\n",
	}

	base := EstimateTimeout(noChecks, nil, models.AgentTypeCodeGen, "default")
	boosted := EstimateTimeout(withChecks, nil, models.AgentTypeCodeGen, "default")

	if boosted <= base {
		t.Errorf("checkbox timeout %v should exceed base %v", boosted, base)
	}
}

func TestEstimateTimeout_FileRefsIncreaseTimeout(t *testing.T) {
	noRefs := &forge.Issue{Number: 1, Title: "test", Body: strings.Repeat("word ", 210)}
	withRefs := &forge.Issue{
		Number: 2,
		Title:  "test",
		Body:   strings.Repeat("word ", 210) + " internal/api/status.go pkg/models/issue.go cmd/samverk/main.go",
	}

	base := EstimateTimeout(noRefs, nil, models.AgentTypeCodeGen, "default")
	boosted := EstimateTimeout(withRefs, nil, models.AgentTypeCodeGen, "default")

	if boosted <= base {
		t.Errorf("file ref timeout %v should exceed base %v", boosted, base)
	}
}

func TestEstimateTimeout_ResearchAgentGetsMore(t *testing.T) {
	issue := &forge.Issue{Number: 1, Title: "test", Body: strings.Repeat("word ", 210)}

	codeGen := EstimateTimeout(issue, nil, models.AgentTypeCodeGen, "default")
	research := EstimateTimeout(issue, nil, models.AgentTypeResearch, "default")

	if research <= codeGen {
		t.Errorf("research timeout %v should exceed code-gen %v", research, codeGen)
	}
}

func TestEstimateTimeout_DocsAgentGetsLess(t *testing.T) {
	issue := &forge.Issue{Number: 1, Title: "test", Body: strings.Repeat("word ", 210)}

	codeGen := EstimateTimeout(issue, nil, models.AgentTypeCodeGen, "default")
	docs := EstimateTimeout(issue, nil, models.AgentTypeDocs, "default")

	if docs >= codeGen {
		t.Errorf("docs timeout %v should be less than code-gen %v", docs, codeGen)
	}
}

func TestEstimateTimeout_ComplexProviderGetsMore(t *testing.T) {
	issue := &forge.Issue{Number: 1, Title: "test", Body: strings.Repeat("word ", 210)}

	def := EstimateTimeout(issue, nil, models.AgentTypeCodeGen, "default")
	comp := EstimateTimeout(issue, nil, models.AgentTypeCodeGen, "complex")

	if comp <= def {
		t.Errorf("complex timeout %v should exceed default %v", comp, def)
	}
}

func TestEstimateTimeout_HighComplexityLabel(t *testing.T) {
	issue := &forge.Issue{
		Number: 1,
		Title:  "test",
		Body:   "short",
		Labels: []string{"complexity:high"},
	}

	got := EstimateTimeout(issue, nil, models.AgentTypeCodeGen, "default")
	if got < 30*time.Minute {
		t.Errorf("got %v, want >= 30m for complexity:high", got)
	}
}

func TestCountCheckboxes(t *testing.T) {
	tests := []struct {
		name string
		body string
		want int
	}{
		{"empty", "", 0},
		{"no checkboxes", "plain text\nmore text", 0},
		{"unchecked", "- [ ] todo one\n- [ ] todo two", 2},
		{"checked", "- [x] done\n- [X] also done", 2},
		{"mixed", "- [ ] todo\n- [x] done\nplain line", 2},
		{"indented", "  - [ ] indented", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := countCheckboxes(tt.body); got != tt.want {
				t.Errorf("countCheckboxes() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestCountFileRefs(t *testing.T) {
	tests := []struct {
		name string
		body string
		want int
	}{
		{"empty", "", 0},
		{"no refs", "plain text without file paths", 0},
		{"single ref", "see internal/api/status.go for details", 1},
		{"multiple refs", "internal/api/status.go and pkg/models/issue.go and cmd/samverk/main.go", 3},
		{"duplicate ref", "internal/api/status.go appears twice internal/api/status.go", 1},
		{"backtick wrapped", "`internal/api/status.go`", 1},
		{"web prefix", "web/src/App.tsx is the entry", 1},
		{"scripts prefix", "scripts/deploy.sh runs it", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := countFileRefs(tt.body); got != tt.want {
				t.Errorf("countFileRefs() = %d, want %d", got, tt.want)
			}
		})
	}
}

// calibrationMockStore is a minimal mock implementing only GetTaskProfile
// for testing CalibratedTimeout. All other Store methods panic if called.
type calibrationMockStore struct {
	store.Store // embed interface to satisfy remaining methods (will panic if called)
	profile     *models.TaskProfile
	err         error
}

func (m *calibrationMockStore) GetTaskProfile(_ context.Context, _, _ string) (*models.TaskProfile, error) {
	return m.profile, m.err
}

func TestCalibratedTimeout_FallbackWhenNoData(t *testing.T) {
	st := &calibrationMockStore{profile: nil}
	issue := &forge.Issue{Number: 1, Title: "test", Body: "short body"}
	logger := zap.NewNop()

	got := CalibratedTimeout(context.Background(), st, logger, issue, nil, models.AgentTypeCodeGen, "default")
	heuristic := EstimateTimeout(issue, nil, models.AgentTypeCodeGen, "default")

	if got != heuristic {
		t.Errorf("with no profile, got %v, want heuristic %v", got, heuristic)
	}
}

func TestCalibratedTimeout_FallbackWhenTooFewSamples(t *testing.T) {
	st := &calibrationMockStore{
		profile: &models.TaskProfile{
			SampleCount: 5,
			P90Duration: 10 * time.Minute,
		},
	}
	issue := &forge.Issue{Number: 1, Title: "test", Body: "short body"}
	logger := zap.NewNop()

	got := CalibratedTimeout(context.Background(), st, logger, issue, nil, models.AgentTypeCodeGen, "default")
	heuristic := EstimateTimeout(issue, nil, models.AgentTypeCodeGen, "default")

	if got != heuristic {
		t.Errorf("with %d samples (< %d), got %v, want heuristic %v", 5, minCalibrationSamples, got, heuristic)
	}
}

func TestCalibratedTimeout_UsesHistoricalWhenEnoughSamples(t *testing.T) {
	p90 := 12 * time.Minute
	st := &calibrationMockStore{
		profile: &models.TaskProfile{
			SampleCount: 25,
			P90Duration: p90,
		},
	}
	issue := &forge.Issue{Number: 1, Title: "test", Body: "short body"}
	logger := zap.NewNop()

	got := CalibratedTimeout(context.Background(), st, logger, issue, nil, models.AgentTypeCodeGen, "default")
	want := time.Duration(float64(p90) * calibrationSafetyMargin)

	if got != want {
		t.Errorf("with %d samples, got %v, want calibrated %v (p90 %v * %.1f)", 25, got, want, p90, calibrationSafetyMargin)
	}
}

func TestCalibratedTimeout_ClampsToMax(t *testing.T) {
	st := &calibrationMockStore{
		profile: &models.TaskProfile{
			SampleCount: 30,
			P90Duration: 55 * time.Minute, // 55 * 1.2 = 66min > MaxTimeout
		},
	}
	issue := &forge.Issue{Number: 1, Title: "test", Body: "short body"}
	logger := zap.NewNop()

	got := CalibratedTimeout(context.Background(), st, logger, issue, nil, models.AgentTypeCodeGen, "default")
	if got != MaxTimeout {
		t.Errorf("got %v, want MaxTimeout %v", got, MaxTimeout)
	}
}

func TestCalibratedTimeout_FrontmatterOverrideStillWins(t *testing.T) {
	st := &calibrationMockStore{
		profile: &models.TaskProfile{
			SampleCount: 30,
			P90Duration: 20 * time.Minute,
		},
	}
	issue := &forge.Issue{Number: 1, Title: "test", Body: "short body"}
	fm := &models.IssueFrontmatter{TimeoutMinutes: 45}
	logger := zap.NewNop()

	got := CalibratedTimeout(context.Background(), st, logger, issue, fm, models.AgentTypeCodeGen, "default")
	if got != 45*time.Minute {
		t.Errorf("got %v, want 45m (frontmatter override)", got)
	}
}

func TestClampTimeout(t *testing.T) {
	tests := []struct {
		name string
		d    time.Duration
		want time.Duration
	}{
		{"below min", 1 * time.Minute, MinTimeout},
		{"at min", MinTimeout, MinTimeout},
		{"at max", MaxTimeout, MaxTimeout},
		{"above max", 120 * time.Minute, MaxTimeout},
		{"in range", 20 * time.Minute, 20 * time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := clampTimeout(tt.d); got != tt.want {
				t.Errorf("clampTimeout(%v) = %v, want %v", tt.d, got, tt.want)
			}
		})
	}
}
