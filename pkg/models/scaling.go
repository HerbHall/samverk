package models

import "time"

// ScalingEvent records a single autoscaler decision.
type ScalingEvent struct {
	ID          string
	Timestamp   time.Time
	Action      string // "scale-up" | "scale-down" | "manual-override"
	FromWorkers int
	ToWorkers   int
	Reason      string
	Confidence  float64
}

// ScalingControl is a single-row record that holds manual override state written
// by the REST API or CLI and consumed by the autoscaler on each evaluation cycle.
// ManualWorkers == 0 means no manual target is set.
// Paused == true means the autoscaler should hold (take no autonomous action).
type ScalingControl struct {
	Paused        bool
	ManualWorkers int
	SetAt         time.Time
	Note          string
}
