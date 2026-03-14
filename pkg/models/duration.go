package models

import (
	"fmt"
	"time"
)

// FormatDuration returns a human-friendly string for a time.Duration.
// Examples: "just now", "5m", "2h", "3d".
func FormatDuration(d time.Duration) string {
	hours := int(d.Hours())
	if hours < 1 {
		mins := int(d.Minutes())
		if mins < 1 {
			return "just now"
		}
		return fmt.Sprintf("%dm", mins)
	}
	if hours < 48 {
		return fmt.Sprintf("%dh", hours)
	}
	days := hours / 24
	return fmt.Sprintf("%dd", days)
}
