package dispatcher

import (
	"errors"
	"testing"
)

func TestTopologicalSort(t *testing.T) {
	tests := []struct {
		name      string
		graph     map[int][]int
		wantErr   error
		wantLen   int
		wantFirst int // expected first element (-1 to skip check)
		wantLast  int // expected last element (-1 to skip check)
		validate  func(t *testing.T, result []int)
	}{
		{
			name:    "empty graph",
			graph:   map[int][]int{},
			wantLen: 0,
		},
		{
			name: "single node no deps",
			graph: map[int][]int{
				1: {},
			},
			wantLen:   1,
			wantFirst: 1,
			wantLast:  1,
		},
		{
			name: "linear chain 1->2->3",
			graph: map[int][]int{
				3: {2},
				2: {1},
			},
			wantLen:   3,
			wantFirst: 1,
			wantLast:  3,
		},
		{
			name: "diamond 4->[2,3] 2->[1] 3->[1]",
			graph: map[int][]int{
				4: {2, 3},
				2: {1},
				3: {1},
			},
			wantLen:   4,
			wantFirst: 1,
			wantLast:  4,
			validate: func(t *testing.T, result []int) {
				t.Helper()
				// 1 must come before 2 and 3; 2 and 3 must come before 4.
				pos := indexMap(result)
				if pos[1] >= pos[2] || pos[1] >= pos[3] {
					t.Errorf("1 must precede 2 and 3: got %v", result)
				}
				if pos[2] >= pos[4] || pos[3] >= pos[4] {
					t.Errorf("2 and 3 must precede 4: got %v", result)
				}
			},
		},
		{
			name: "forest disconnected components",
			graph: map[int][]int{
				2: {1},
				4: {3},
			},
			wantLen: 4,
			validate: func(t *testing.T, result []int) {
				t.Helper()
				pos := indexMap(result)
				if pos[1] >= pos[2] {
					t.Errorf("1 must precede 2: got %v", result)
				}
				if pos[3] >= pos[4] {
					t.Errorf("3 must precede 4: got %v", result)
				}
			},
		},
		{
			name: "cycle 1<->2",
			graph: map[int][]int{
				1: {2},
				2: {1},
			},
			wantErr: ErrCycleDetected,
		},
		{
			name: "three node cycle",
			graph: map[int][]int{
				1: {3},
				2: {1},
				3: {2},
			},
			wantErr: ErrCycleDetected,
		},
		{
			name: "wide parallel deps",
			graph: map[int][]int{
				10: {1, 2, 3, 4, 5},
			},
			wantLen:  6,
			wantLast: 10,
			validate: func(t *testing.T, result []int) {
				t.Helper()
				pos := indexMap(result)
				for _, dep := range []int{1, 2, 3, 4, 5} {
					if pos[dep] >= pos[10] {
						t.Errorf("dep %d must precede 10: got %v", dep, result)
					}
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := TopologicalSort(tt.graph)

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("want error %v, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.wantLen > 0 && len(result) != tt.wantLen {
				t.Fatalf("want length %d, got %d: %v", tt.wantLen, len(result), result)
			}
			if tt.wantLen == 0 && len(result) != 0 {
				t.Fatalf("want empty result, got %v", result)
			}

			if tt.wantFirst > 0 && len(result) > 0 && result[0] != tt.wantFirst {
				t.Errorf("want first element %d, got %d: %v", tt.wantFirst, result[0], result)
			}
			if tt.wantLast > 0 && len(result) > 0 && result[len(result)-1] != tt.wantLast {
				t.Errorf("want last element %d, got %d: %v", tt.wantLast, result[len(result)-1], result)
			}

			if tt.validate != nil {
				tt.validate(t, result)
			}
		})
	}
}

func TestCriticalPath(t *testing.T) {
	tests := []struct {
		name     string
		graph    map[int][]int
		wantLen  int
		wantPath []int // exact path if deterministic, nil to skip
		validate func(t *testing.T, path []int)
	}{
		{
			name:    "empty graph",
			graph:   map[int][]int{},
			wantLen: 0,
		},
		{
			name: "single node",
			graph: map[int][]int{
				1: {},
			},
			wantLen:  1,
			wantPath: []int{1},
		},
		{
			name: "linear chain 1->2->3",
			graph: map[int][]int{
				3: {2},
				2: {1},
			},
			wantLen:  3,
			wantPath: []int{1, 2, 3},
		},
		{
			name: "diamond longest path length 3",
			graph: map[int][]int{
				4: {2, 3},
				2: {1},
				3: {1},
			},
			wantLen: 3,
			validate: func(t *testing.T, path []int) {
				t.Helper()
				// Must start at 1 and end at 4.
				if path[0] != 1 {
					t.Errorf("path should start at 1, got %v", path)
				}
				if path[len(path)-1] != 4 {
					t.Errorf("path should end at 4, got %v", path)
				}
			},
		},
		{
			name: "wide no chain beyond 2",
			graph: map[int][]int{
				2: {1},
				3: {1},
				4: {1},
			},
			wantLen: 2,
			validate: func(t *testing.T, path []int) {
				t.Helper()
				if path[0] != 1 {
					t.Errorf("path should start at 1, got %v", path)
				}
			},
		},
		{
			name: "longer branch wins",
			graph: map[int][]int{
				5: {4},
				4: {3},
				3: {1},
				2: {1},
			},
			wantLen:  4,
			wantPath: []int{1, 3, 4, 5},
		},
		{
			name: "cycle returns nil",
			graph: map[int][]int{
				1: {2},
				2: {1},
			},
			wantLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CriticalPath(tt.graph)

			if tt.wantLen == 0 {
				if result != nil {
					t.Fatalf("want nil, got %v", result)
				}
				return
			}

			if len(result) != tt.wantLen {
				t.Fatalf("want path length %d, got %d: %v", tt.wantLen, len(result), result)
			}

			if tt.wantPath != nil {
				for i := range tt.wantPath {
					if result[i] != tt.wantPath[i] {
						t.Errorf("path[%d]: want %d, got %d (full: %v)", i, tt.wantPath[i], result[i], result)
					}
				}
			}

			if tt.validate != nil {
				tt.validate(t, result)
			}
		})
	}
}

func TestReadyIssues(t *testing.T) {
	tests := []struct {
		name      string
		graph     map[int][]int
		completed map[int]bool
		want      []int
	}{
		{
			name:      "empty graph empty completed",
			graph:     map[int][]int{},
			completed: map[int]bool{},
			want:      []int{},
		},
		{
			name: "all deps completed returns dependent",
			graph: map[int][]int{
				3: {1, 2},
			},
			completed: map[int]bool{1: true, 2: true},
			want:      []int{3},
		},
		{
			name: "some deps incomplete excludes dependent",
			graph: map[int][]int{
				3: {1, 2},
			},
			completed: map[int]bool{1: true},
			want:      []int{2},
		},
		{
			name: "no deps always ready if not completed",
			graph: map[int][]int{
				1: {},
				2: {},
			},
			completed: map[int]bool{},
			want:      []int{1, 2},
		},
		{
			name: "already completed excluded",
			graph: map[int][]int{
				1: {},
				2: {1},
			},
			completed: map[int]bool{1: true},
			want:      []int{2},
		},
		{
			name: "chain with middle completed",
			graph: map[int][]int{
				3: {2},
				2: {1},
			},
			completed: map[int]bool{1: true, 2: true},
			want:      []int{3},
		},
		{
			name: "nothing ready when deps incomplete",
			graph: map[int][]int{
				2: {1},
				3: {2},
			},
			completed: map[int]bool{},
			want:      []int{1},
		},
		{
			name: "diamond with partial completion",
			graph: map[int][]int{
				4: {2, 3},
				2: {1},
				3: {1},
			},
			completed: map[int]bool{1: true},
			want:      []int{2, 3},
		},
		{
			name: "nil completed treated as empty",
			graph: map[int][]int{
				2: {1},
			},
			completed: nil,
			want:      []int{1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ReadyIssues(tt.graph, tt.completed)

			if len(result) != len(tt.want) {
				t.Fatalf("want %v, got %v", tt.want, result)
			}
			for i := range tt.want {
				if result[i] != tt.want[i] {
					t.Errorf("result[%d]: want %d, got %d (full: %v)", i, tt.want[i], result[i], result)
				}
			}
		})
	}
}

// indexMap returns a map from value to index for a slice.
func indexMap(s []int) map[int]int {
	m := make(map[int]int, len(s))
	for i, v := range s {
		m[v] = i
	}
	return m
}
