package models

import (
	"testing"
	"time"
)

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name string
		d    time.Duration
		want string
	}{
		{name: "zero", d: 0, want: "just now"},
		{name: "30 seconds", d: 30 * time.Second, want: "just now"},
		{name: "5 minutes", d: 5 * time.Minute, want: "5m"},
		{name: "1 hour", d: time.Hour, want: "1h"},
		{name: "23 hours", d: 23 * time.Hour, want: "23h"},
		{name: "48 hours", d: 48 * time.Hour, want: "2d"},
		{name: "72 hours", d: 72 * time.Hour, want: "3d"},
		{name: "7 days", d: 7 * 24 * time.Hour, want: "7d"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatDuration(tt.d)
			if got != tt.want {
				t.Errorf("FormatDuration(%v) = %q, want %q", tt.d, got, tt.want)
			}
		})
	}
}
