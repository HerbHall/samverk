package dispatcher

import (
	"errors"
	"sort"
)

// ErrCycleDetected is returned by TopologicalSort when the graph contains a cycle.
var ErrCycleDetected = errors.New("dependency cycle detected")

// TopologicalSort returns issue numbers in dependency-respecting order
// using Kahn's algorithm. Returns an error if the graph contains a cycle.
//
// The graph maps each issue to its dependencies: graph[5] = [3, 4] means
// issue #5 depends on #3 and #4. The returned order places dependencies
// before their dependents (3 and 4 before 5).
func TopologicalSort(graph map[int][]int) ([]int, error) {
	if len(graph) == 0 {
		return nil, nil
	}

	// Collect all unique nodes (both keys and dependency values).
	nodeSet := make(map[int]bool)
	for node, deps := range graph {
		nodeSet[node] = true
		for _, dep := range deps {
			nodeSet[dep] = true
		}
	}

	// Build forward adjacency list: forward[dep] = list of nodes that depend on dep.
	// Compute in-degree: number of dependencies each node has.
	forward := make(map[int][]int, len(nodeSet))
	inDegree := make(map[int]int, len(nodeSet))
	for node := range nodeSet {
		inDegree[node] = 0
	}
	for node, deps := range graph {
		inDegree[node] = len(deps)
		for _, dep := range deps {
			forward[dep] = append(forward[dep], node)
		}
	}

	// Initialize queue with all zero in-degree nodes, sorted for determinism.
	queue := make([]int, 0, len(nodeSet))
	for node, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, node)
		}
	}
	sort.Ints(queue)

	result := make([]int, 0, len(nodeSet))
	for len(queue) > 0 {
		// Pop from front.
		node := queue[0]
		queue = queue[1:]
		result = append(result, node)

		// Collect successors and sort for deterministic output.
		successors := make([]int, len(forward[node]))
		copy(successors, forward[node])
		sort.Ints(successors)

		for _, succ := range successors {
			inDegree[succ]--
			if inDegree[succ] == 0 {
				queue = insertSorted(queue, succ)
			}
		}
	}

	if len(result) != len(nodeSet) {
		return nil, ErrCycleDetected
	}

	return result, nil
}

// CriticalPath returns the longest dependency chain in the graph.
// Uses topological ordering with dynamic programming to find the longest
// path. Returns nil for empty or cyclic graphs.
func CriticalPath(graph map[int][]int) []int {
	order, err := TopologicalSort(graph)
	if err != nil || len(order) == 0 {
		return nil
	}

	// dist[node] = length of longest path ending at node.
	// pred[node] = predecessor on the longest path.
	dist := make(map[int]int, len(order))
	pred := make(map[int]int, len(order))
	for _, node := range order {
		dist[node] = 1
		pred[node] = -1
	}

	// Process in topological order. For each node, update its dependents.
	// forward[dep] = nodes that depend on dep (dep must come before them).
	forward := make(map[int][]int, len(order))
	for node, deps := range graph {
		for _, dep := range deps {
			forward[dep] = append(forward[dep], node)
		}
	}

	for _, node := range order {
		for _, succ := range forward[node] {
			candidate := dist[node] + 1
			if candidate > dist[succ] {
				dist[succ] = candidate
				pred[succ] = node
			}
		}
	}

	// Find the node with the maximum distance.
	maxDist := 0
	endNode := -1
	for _, node := range order {
		if dist[node] > maxDist {
			maxDist = dist[node]
			endNode = node
		}
	}

	if endNode == -1 {
		return nil
	}

	// Reconstruct the path by following predecessors.
	path := make([]int, 0, maxDist)
	for cur := endNode; cur != -1; cur = pred[cur] {
		path = append(path, cur)
	}

	// Reverse the path so it goes from start to end.
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}

	return path
}

// ReadyIssues returns issue numbers whose dependencies are all in the
// completed set (or have no dependencies). Issues already in completed
// are excluded from the result. The result is sorted for deterministic output.
func ReadyIssues(graph map[int][]int, completed map[int]bool) []int {
	// Collect all nodes in the graph.
	nodeSet := make(map[int]bool)
	for node, deps := range graph {
		nodeSet[node] = true
		for _, dep := range deps {
			nodeSet[dep] = true
		}
	}

	ready := make([]int, 0)
	for node := range nodeSet {
		if completed[node] {
			continue
		}

		deps := graph[node]
		allDepsCompleted := true
		for _, dep := range deps {
			if !completed[dep] {
				allDepsCompleted = false
				break
			}
		}

		if allDepsCompleted {
			ready = append(ready, node)
		}
	}

	sort.Ints(ready)
	return ready
}

// insertSorted inserts val into a sorted slice maintaining sorted order.
func insertSorted(sorted []int, val int) []int {
	idx := sort.SearchInts(sorted, val)
	sorted = append(sorted, 0)
	copy(sorted[idx+1:], sorted[idx:])
	sorted[idx] = val
	return sorted
}
