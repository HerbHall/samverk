package models

import "time"

// ScalingEvent records a single autoscaler decision.
type ScalingEvent struct {
	ID          string
	Timestamp   time.Time
	Action      string // "scale_up" | "scale_down"
	FromWorkers int
	ToWorkers   int
	Reason      string
	Confidence  float64
}
