package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"
	"time"

	"github.com/herbhall/samverk/internal/api"
	"github.com/herbhall/samverk/internal/audit"
	"github.com/herbhall/samverk/internal/digest"
	"github.com/herbhall/samverk/internal/forge"
	"github.com/herbhall/samverk/internal/provider"
	"github.com/herbhall/samverk/internal/store"
	"github.com/herbhall/samverk/pkg/models"
)

// mockTracker implements forge.IssueTracker for testing.
type mockTracker struct {
	issues   []*forge.Issue
	issueMap map[int]*forge.Issue
	listErr  error
	getErr   error
}

func newMockTracker(issues ...*forge.Issue) *mockTracker {
	m := &mockTracker{
		issues:   issues,
		issueMap: make(map[int]*forge.Issue),
	}
	for _, iss := range issues {
		m.issueMap[iss.Number] = iss
	}
	return m
}

func (m *mockTracker) ListIssues(_ context.Context, _ *forge.ListOptions) ([]*forge.Issue, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.issues, nil
}

func (m *mockTracker) GetIssue(_ context.Context, number int) (*forge.Issue, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	iss, ok := m.issueMap[number]
	if !ok {
		return nil, errors.New("not found")
	}
	return iss, nil
}

func (m *mockTracker) CreateIssue(_ context.Context, _ *forge.CreateIssueRequest) (*forge.Issue, error) {
	return nil, errors.New("not implemented")
}

func (m *mockTracker) UpdateIssue(_ context.Context, _ int, _ *forge.UpdateIssueRequest) (*forge.Issue, error) {
	return nil, errors.New("not implemented")
}

func (m *mockTracker) AddComment(_ context.Context, _ int, _ string) (*forge.Comment, error) {
	return nil, errors.New("not implemented")
}

func (m *mockTracker) ListComments(_ context.Context, _ int) ([]*forge.Comment, error) {
	return nil, errors.New("not implemented")
}

func (m *mockTracker) SetLabels(_ context.Context, _ int, _ []string) error {
	return errors.New("not implemented")
}

func (m *mockTracker) AddLabel(_ context.Context, _ int, _ string) error {
	return errors.New("not implemented")
}

func (m *mockTracker) RemoveLabel(_ context.Context, _ int, _ string) error {
	return errors.New("not implemented")
}

func (m *mockTracker) Assign(_ context.Context, _ int, _ string) error {
	return errors.New("not implemented")
}

func (m *mockTracker) Unassign(_ context.Context, _ int, _ string) error {
	return errors.New("not implemented")
}

func (m *mockTracker) Watch(_ context.Context, _ func(forge.Event)) error {
	return errors.New("not implemented")
}

func (m *mockTracker) SearchIssues(_ context.Context, _ *forge.SearchOptions) ([]*forge.Issue, error) {
	return nil, errors.New("not implemented")
}

// mockStore implements store.Store for testing.
type mockStore struct {
	sessions []*models.Session
	listErr  error
}

func (m *mockStore) CreateSession(_ context.Context, _ *models.Session) error {
	return errors.New("not implemented")
}

func (m *mockStore) GetSession(_ context.Context, _ string) (*models.Session, error) {
	return nil, errors.New("not implemented")
}

func (m *mockStore) UpdateSession(_ context.Context, _ *models.Session) error {
	return errors.New("not implemented")
}

func (m *mockStore) ListSessions(_ context.Context, _ models.SessionStatus) ([]*models.Session, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.sessions, nil
}

func (m *mockStore) RecordCost(_ context.Context, _ *models.CostRecord) error {
	return errors.New("not implemented")
}

func (m *mockStore) ComputeCostSince(_ context.Context, _ time.Time) (*models.CostSummary, error) {
	return nil, errors.New("not implemented")
}

func (m *mockStore) ComputeCostForIssue(_ context.Context, _ int) (*models.CostSummary, error) {
	return &models.CostSummary{}, nil
}

func (m *mockStore) GetBudgetStatus(_ context.Context, _ float64) (spent, remaining float64, err error) {
	return 0, 0, errors.New("not implemented")
}

func (m *mockStore) SaveScalingEvent(_ context.Context, _ models.ScalingEvent) error {
	return errors.New("not implemented")
}

func (m *mockStore) ListScalingEvents(_ context.Context, _ int) ([]*models.ScalingEvent, error) {
	return nil, nil
}

func (m *mockStore) GetScalingControl(_ context.Context) (*models.ScalingControl, error) {
	return &models.ScalingControl{}, nil
}

func (m *mockStore) UpsertScalingControl(_ context.Context, _ models.ScalingControl) error {
	return nil
}

func (m *mockStore) UpdateTaskProfile(_ context.Context, _, _ string) error {
	return nil
}

func (m *mockStore) ListTaskProfiles(_ context.Context) ([]*models.TaskProfile, error) {
	return nil, nil
}

func (m *mockStore) GetTaskProfile(_ context.Context, _, _ string) (*models.TaskProfile, error) {
	return nil, nil
}

func (m *mockStore) SaveMetricSnapshot(_ context.Context, _ store.MetricSnapshot) error {
	return nil
}

func (m *mockStore) LatestMetricSnapshot(_ context.Context) (*store.MetricSnapshot, error) {
	return nil, errors.New("no snapshot")
}

func (m *mockStore) SaveFailureEvent(_ context.Context, _ *models.FailureEvent) error { return nil }
func (m *mockStore) ListFailureEvents(_ context.Context, _ time.Time, _ int) ([]*models.FailureEvent, error) {
	return nil, nil
}
func (m *mockStore) CountFailuresByIssue(_ context.Context, _ int) (int, error) { return 0, nil }
func (m *mockStore) GetFailureSummary(_ context.Context, _ time.Time) (*models.FailureSummary, error) {
	return &models.FailureSummary{ByClass: map[models.FailureClass]int{}}, nil
}
func (m *mockStore) GetIssueFailureCount(_ context.Context, _ int) (int, error)      { return 0, nil }
func (m *mockStore) IncrementIssueFailureCount(_ context.Context, _ int) (int, error) { return 1, nil }
func (m *mockStore) ClearIssueFailureCount(_ context.Context, _ int) error            { return nil }
func (m *mockStore) SaveCorrectionEvent(_ context.Context, _ *models.CorrectionEvent) error {
	return nil
}
func (m *mockStore) ListCorrectionEvents(_ context.Context, _ int) ([]*models.CorrectionEvent, error) {
	return nil, nil
}
func (m *mockStore) SaveAuditResults(_ context.Context, _ []audit.AuditResult) error {
	return nil
}
func (m *mockStore) GetLastAudit(_ context.Context) ([]audit.AuditResult, error) {
	return nil, nil
}

func (m *mockStore) GetLastCheckIn(_ context.Context) (time.Time, error) { return time.Time{}, nil }
func (m *mockStore) SetLastCheckIn(_ context.Context, _ time.Time) error { return nil }
func (m *mockStore) SaveProviderHealthSnapshot(_ context.Context, _ []provider.ProviderHealth) error {
	return nil
}
func (m *mockStore) LoadProviderHealthSnapshot(_ context.Context) ([]provider.ProviderHealth, time.Time, error) {
	return nil, time.Time{}, store.ErrNotFound
}
func (m *mockStore) LatestSuccessByProvider(_ context.Context) (map[string]time.Time, error) {
	return nil, nil
}
func (m *mockStore) GetKPIReport(_ context.Context) (*models.KPIReport, error) {
	return &models.KPIReport{
		WindowDays:         30,
		RootCauseBreakdown: []models.RootCauseBreakdownEntry{},
	}, nil
}
func (m *mockStore) Close() error { return nil }

// Compile-time check: mockStore implements store.Store.
var _ store.Store = (*mockStore)(nil)

// mockCosts implements digest.CostSource for testing.
type mockCosts struct {
	summary digest.CostSummary
}

func (m *mockCosts) ComputeCostSince(_ context.Context, _ time.Time) digest.CostSummary {
	return m.summary
}

// newTestAPI creates an API with all mocks wired and returns an httptest.Server.
func newTestAPI(t *testing.T, tracker forge.IssueTracker, s store.Store, costs digest.CostSource) *httptest.Server {
	t.Helper()
	a := api.New(tracker, s, costs, zap.NewNop())
	mux := http.NewServeMux()
	a.RegisterRoutes(mux)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts
}

// doGet performs a GET request with context and returns the response.
func doGet(t *testing.T, url string) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, http.NoBody)
	if err != nil {
		t.Fatalf("new GET request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	return resp
}

// decodeJSON decodes the response body into the given target.
func decodeJSON(t *testing.T, resp *http.Response, target any) {
	t.Helper()
	defer func() { _ = resp.Body.Close() }()
	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}

func TestListIssues(t *testing.T) {
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	tracker := newMockTracker(
		&forge.Issue{Number: 1, Title: "First issue", State: forge.StateOpen, CreatedAt: now, UpdatedAt: now},
		&forge.Issue{Number: 2, Title: "Second issue", State: forge.StateOpen, Labels: []string{"bug"}, CreatedAt: now, UpdatedAt: now},
	)

	tests := []struct {
		name       string
		tracker    forge.IssueTracker
		path       string
		wantStatus int
		wantLen    int
		wantError  string
	}{
		{
			name:       "success with issues",
			tracker:    tracker,
			path:       "/api/v1/issues",
			wantStatus: http.StatusOK,
			wantLen:    2,
		},
		{
			name:       "with query params",
			tracker:    tracker,
			path:       "/api/v1/issues?state=open&labels=bug&page=1&per_page=10",
			wantStatus: http.StatusOK,
			wantLen:    2,
		},
		{
			name:       "nil tracker returns 503",
			tracker:    nil,
			path:       "/api/v1/issues",
			wantStatus: http.StatusServiceUnavailable,
			wantError:  "issue tracker not available",
		},
		{
			name:       "tracker error returns 500",
			tracker:    &mockTracker{listErr: errors.New("api rate limit")},
			path:       "/api/v1/issues",
			wantStatus: http.StatusInternalServerError,
			wantError:  "failed to list issues",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := newTestAPI(t, tt.tracker, nil, nil)

			resp := doGet(t, ts.URL+tt.path)
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != tt.wantStatus {
				t.Errorf("status = %d, want %d", resp.StatusCode, tt.wantStatus)
			}
			if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
				t.Errorf("Content-Type = %q, want %q", ct, "application/json")
			}

			if tt.wantError != "" {
				var body map[string]string
				decodeJSON(t, resp, &body)
				if body["error"] != tt.wantError {
					t.Errorf("error = %q, want %q", body["error"], tt.wantError)
				}
				return
			}

			var body map[string]any
			decodeJSON(t, resp, &body)
			issues, _ := body["issues"].([]any)
			if len(issues) != tt.wantLen {
				t.Errorf("issue count = %d, want %d", len(issues), tt.wantLen)
			}
		})
	}
}

func TestGetIssue(t *testing.T) {
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	tracker := newMockTracker(
		&forge.Issue{Number: 42, Title: "Test issue", Body: "Some body", State: forge.StateOpen, CreatedAt: now, UpdatedAt: now},
	)

	tests := []struct {
		name       string
		tracker    forge.IssueTracker
		path       string
		wantStatus int
		wantTitle  string
		wantError  string
	}{
		{
			name:       "existing issue",
			tracker:    tracker,
			path:       "/api/v1/issues/42",
			wantStatus: http.StatusOK,
			wantTitle:  "Test issue",
		},
		{
			name:       "not found",
			tracker:    tracker,
			path:       "/api/v1/issues/999",
			wantStatus: http.StatusNotFound,
			wantError:  "issue not found",
		},
		{
			name:       "invalid number",
			tracker:    tracker,
			path:       "/api/v1/issues/abc",
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid issue number",
		},
		{
			name:       "zero number",
			tracker:    tracker,
			path:       "/api/v1/issues/0",
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid issue number",
		},
		{
			name:       "nil tracker",
			tracker:    nil,
			path:       "/api/v1/issues/1",
			wantStatus: http.StatusServiceUnavailable,
			wantError:  "issue tracker not available",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := newTestAPI(t, tt.tracker, nil, nil)

			resp := doGet(t, ts.URL+tt.path)
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != tt.wantStatus {
				t.Errorf("status = %d, want %d", resp.StatusCode, tt.wantStatus)
			}

			if tt.wantError != "" {
				var body map[string]string
				decodeJSON(t, resp, &body)
				if body["error"] != tt.wantError {
					t.Errorf("error = %q, want %q", body["error"], tt.wantError)
				}
				return
			}

			var issue map[string]any
			decodeJSON(t, resp, &issue)
			if title, ok := issue["title"].(string); !ok || title != tt.wantTitle {
				t.Errorf("title = %v, want %q", issue["title"], tt.wantTitle)
			}
		})
	}
}

func TestListSessions(t *testing.T) {
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	ms := &mockStore{
		sessions: []*models.Session{
			{ID: "s1", IssueNumber: 1, AgentType: "code-gen", Provider: "ollama", Model: "qwen2.5:7b", Status: models.SessionStatusActive, StartedAt: now, CreatedAt: now, UpdatedAt: now},
		},
	}

	tests := []struct {
		name       string
		store      store.Store
		path       string
		wantStatus int
		wantLen    int
	}{
		{
			name:       "with sessions",
			store:      ms,
			path:       "/api/v1/sessions",
			wantStatus: http.StatusOK,
			wantLen:    1,
		},
		{
			name:       "with status filter",
			store:      ms,
			path:       "/api/v1/sessions?status=active",
			wantStatus: http.StatusOK,
			wantLen:    1,
		},
		{
			name:       "nil store returns empty array",
			store:      nil,
			path:       "/api/v1/sessions",
			wantStatus: http.StatusOK,
			wantLen:    0,
		},
		{
			name:       "store error returns 500",
			store:      &mockStore{listErr: errors.New("db error")},
			path:       "/api/v1/sessions",
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := newTestAPI(t, nil, tt.store, nil)

			resp := doGet(t, ts.URL+tt.path)
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != tt.wantStatus {
				t.Errorf("status = %d, want %d", resp.StatusCode, tt.wantStatus)
			}

			if tt.wantStatus == http.StatusOK {
				var sessions []map[string]any
				decodeJSON(t, resp, &sessions)
				if len(sessions) != tt.wantLen {
					t.Errorf("session count = %d, want %d", len(sessions), tt.wantLen)
				}
			}
		})
	}
}

func TestGetCosts(t *testing.T) {
	mc := &mockCosts{
		summary: digest.CostSummary{
			TokensUsed:         50000,
			EstimatedCostUSD:   1.25,
			BudgetRemainingUSD: 8.75,
		},
	}

	tests := []struct {
		name       string
		costs      digest.CostSource
		path       string
		wantStatus int
		wantTokens float64
	}{
		{
			name:       "with cost data",
			costs:      mc,
			path:       "/api/v1/costs",
			wantStatus: http.StatusOK,
			wantTokens: 50000,
		},
		{
			name:       "with since param",
			costs:      mc,
			path:       "/api/v1/costs?since=1h",
			wantStatus: http.StatusOK,
			wantTokens: 50000,
		},
		{
			name:       "nil costs returns zeroed response",
			costs:      nil,
			path:       "/api/v1/costs",
			wantStatus: http.StatusOK,
			wantTokens: 0,
		},
		{
			name:       "invalid since duration",
			costs:      mc,
			path:       "/api/v1/costs?since=invalid",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := newTestAPI(t, nil, nil, tt.costs)

			resp := doGet(t, ts.URL+tt.path)
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != tt.wantStatus {
				t.Errorf("status = %d, want %d", resp.StatusCode, tt.wantStatus)
			}

			if tt.wantStatus == http.StatusOK {
				var body map[string]any
				decodeJSON(t, resp, &body)
				tokens, _ := body["tokens_used"].(float64)
				if tokens != tt.wantTokens {
					t.Errorf("tokens_used = %v, want %v", tokens, tt.wantTokens)
				}
			}
		})
	}
}

func TestStatus(t *testing.T) {
	tests := []struct {
		name        string
		tracker     forge.IssueTracker
		store       store.Store
		costs       digest.CostSource
		wantHealthy bool
		wantForge   bool
		wantDB      bool
	}{
		{
			name:        "all dependencies available",
			tracker:     newMockTracker(),
			store:       &mockStore{},
			costs:       &mockCosts{},
			wantHealthy: true,
			wantForge:   true,
			wantDB:      true,
		},
		{
			name:        "no dependencies",
			tracker:     nil,
			store:       nil,
			costs:       nil,
			wantHealthy: false,
			wantForge:   false,
			wantDB:      false,
		},
		{
			name:        "partial dependencies tracker only",
			tracker:     newMockTracker(),
			store:       nil,
			costs:       nil,
			wantHealthy: false,
			wantForge:   true,
			wantDB:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := newTestAPI(t, tt.tracker, tt.store, tt.costs)

			resp := doGet(t, ts.URL+"/api/v1/status")
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusOK {
				t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
			}

			var body map[string]any
			decodeJSON(t, resp, &body)

			if healthy, ok := body["healthy"].(bool); !ok || healthy != tt.wantHealthy {
				t.Errorf("healthy = %v, want %v", body["healthy"], tt.wantHealthy)
			}
			if forgeConnected, ok := body["forge_connected"].(bool); !ok || forgeConnected != tt.wantForge {
				t.Errorf("forge_connected = %v, want %v", body["forge_connected"], tt.wantForge)
			}
			if dbConnected, ok := body["database_connected"].(bool); !ok || dbConnected != tt.wantDB {
				t.Errorf("database_connected = %v, want %v", body["database_connected"], tt.wantDB)
			}
		})
	}
}

func TestIssueResponseEmptySlices(t *testing.T) {
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	tracker := newMockTracker(
		&forge.Issue{Number: 1, Title: "No labels", State: forge.StateOpen, CreatedAt: now, UpdatedAt: now},
	)
	ts := newTestAPI(t, tracker, nil, nil)

	resp := doGet(t, ts.URL+"/api/v1/issues")
	defer func() { _ = resp.Body.Close() }()

	// Verify that nil slices are serialized as empty arrays, not null.
	var wrapper struct {
		Issues []map[string]json.RawMessage `json:"issues"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&wrapper); err != nil {
		t.Fatalf("decode: %v", err)
	}
	issues := wrapper.Issues
	if len(issues) == 0 {
		t.Fatal("expected at least one issue")
	}

	// labels and assignees should be [] not null.
	for _, field := range []string{"labels", "assignees"} {
		val := string(issues[0][field])
		if val == "null" {
			t.Errorf("%s = null, want empty array []", field)
		}
	}
}

func TestListIssues_DefaultParams(t *testing.T) {
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	tracker := newMockTracker(
		&forge.Issue{Number: 1, Title: "A", State: forge.StateOpen, CreatedAt: now, UpdatedAt: now},
	)
	ts := newTestAPI(t, tracker, nil, nil)

	resp := doGet(t, ts.URL+"/api/v1/issues")
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var body map[string]any
	decodeJSON(t, resp, &body)

	if limit, _ := body["limit"].(float64); limit != 50 {
		t.Errorf("limit = %v, want 50", body["limit"])
	}
	if offset, _ := body["offset"].(float64); offset != 0 {
		t.Errorf("offset = %v, want 0", body["offset"])
	}
	if page, _ := body["page"].(float64); page != 1 {
		t.Errorf("page = %v, want 1", body["page"])
	}
}

func TestListIssues_LimitOffset(t *testing.T) {
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	tracker := newMockTracker(
		&forge.Issue{Number: 1, Title: "A", State: forge.StateOpen, CreatedAt: now, UpdatedAt: now},
	)
	ts := newTestAPI(t, tracker, nil, nil)

	resp := doGet(t, ts.URL+"/api/v1/issues?limit=10&offset=20")
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var body map[string]any
	decodeJSON(t, resp, &body)

	if limit, _ := body["limit"].(float64); limit != 10 {
		t.Errorf("limit = %v, want 10", body["limit"])
	}
	if offset, _ := body["offset"].(float64); offset != 20 {
		t.Errorf("offset = %v, want 20", body["offset"])
	}
	// page = offset/limit + 1 = 20/10 + 1 = 3
	if page, _ := body["page"].(float64); page != 3 {
		t.Errorf("page = %v, want 3", body["page"])
	}
}

func TestListIssues_LimitCap(t *testing.T) {
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	tracker := newMockTracker(
		&forge.Issue{Number: 1, Title: "A", State: forge.StateOpen, CreatedAt: now, UpdatedAt: now},
	)
	ts := newTestAPI(t, tracker, nil, nil)

	resp := doGet(t, ts.URL+"/api/v1/issues?limit=500")
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var body map[string]any
	decodeJSON(t, resp, &body)

	if limit, _ := body["limit"].(float64); limit != 200 {
		t.Errorf("limit = %v, want 200 (capped)", body["limit"])
	}
}

func TestListIssues_StateFilter(t *testing.T) {
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	var capturedOpts *forge.ListOptions
	tracker := &capturingMockTracker{
		issues: []*forge.Issue{
			{Number: 1, Title: "Closed one", State: forge.StateClosed, CreatedAt: now, UpdatedAt: now},
		},
		captureOpts: func(opts *forge.ListOptions) {
			capturedOpts = opts
		},
	}
	ts := newTestAPI(t, tracker, nil, nil)

	resp := doGet(t, ts.URL+"/api/v1/issues?state=closed")
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	if capturedOpts == nil {
		t.Fatal("opts not captured")
	}
	if capturedOpts.State != forge.StateClosed {
		t.Errorf("opts.State = %q, want %q", capturedOpts.State, forge.StateClosed)
	}
}

func TestListIssues_WrappedResponse(t *testing.T) {
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	tracker := newMockTracker(
		&forge.Issue{Number: 1, Title: "A", State: forge.StateOpen, CreatedAt: now, UpdatedAt: now},
	)
	ts := newTestAPI(t, tracker, nil, nil)

	resp := doGet(t, ts.URL+"/api/v1/issues")
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var body map[string]any
	decodeJSON(t, resp, &body)

	// Must have "issues" key with array value.
	issues, ok := body["issues"]
	if !ok {
		t.Fatal("response missing 'issues' key")
	}
	issueArr, ok := issues.([]any)
	if !ok {
		t.Fatalf("'issues' is not an array, got %T", issues)
	}
	if len(issueArr) != 1 {
		t.Errorf("issues count = %d, want 1", len(issueArr))
	}

	// Must have pagination fields.
	for _, field := range []string{"total", "limit", "offset", "page", "per_page"} {
		if _, ok := body[field]; !ok {
			t.Errorf("response missing %q field", field)
		}
	}
}

// capturingMockTracker records the ListOptions passed to ListIssues.
type capturingMockTracker struct {
	issues      []*forge.Issue
	captureOpts func(*forge.ListOptions)
}

func (m *capturingMockTracker) ListIssues(_ context.Context, opts *forge.ListOptions) ([]*forge.Issue, error) {
	if m.captureOpts != nil {
		m.captureOpts(opts)
	}
	return m.issues, nil
}

func (m *capturingMockTracker) GetIssue(_ context.Context, _ int) (*forge.Issue, error) {
	return nil, errors.New("not implemented")
}

func (m *capturingMockTracker) CreateIssue(_ context.Context, _ *forge.CreateIssueRequest) (*forge.Issue, error) {
	return nil, errors.New("not implemented")
}

func (m *capturingMockTracker) UpdateIssue(_ context.Context, _ int, _ *forge.UpdateIssueRequest) (*forge.Issue, error) {
	return nil, errors.New("not implemented")
}

func (m *capturingMockTracker) AddComment(_ context.Context, _ int, _ string) (*forge.Comment, error) {
	return nil, errors.New("not implemented")
}

func (m *capturingMockTracker) ListComments(_ context.Context, _ int) ([]*forge.Comment, error) {
	return nil, errors.New("not implemented")
}

func (m *capturingMockTracker) SetLabels(_ context.Context, _ int, _ []string) error {
	return errors.New("not implemented")
}

func (m *capturingMockTracker) AddLabel(_ context.Context, _ int, _ string) error {
	return errors.New("not implemented")
}

func (m *capturingMockTracker) RemoveLabel(_ context.Context, _ int, _ string) error {
	return errors.New("not implemented")
}

func (m *capturingMockTracker) Assign(_ context.Context, _ int, _ string) error {
	return errors.New("not implemented")
}

func (m *capturingMockTracker) Unassign(_ context.Context, _ int, _ string) error {
	return errors.New("not implemented")
}

func (m *capturingMockTracker) Watch(_ context.Context, _ func(forge.Event)) error {
	return errors.New("not implemented")
}

func (m *capturingMockTracker) SearchIssues(_ context.Context, _ *forge.SearchOptions) ([]*forge.Issue, error) {
	return nil, errors.New("not implemented")
}
