package api

import "time"

// ComputeWorkerStatusForTest exposes computeWorkerStatus to the api_test package.
func ComputeWorkerStatusForTest(lastHeartbeat time.Time, stale, offline time.Duration) string {
	return string(computeWorkerStatus(lastHeartbeat, stale, offline))
}