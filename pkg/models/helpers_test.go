package models

import "testing"

func TestIsEven(t *testing.T) {
	tests := []struct {
		name string
		n    int
		want bool
	}{
		{"zero", 0, true},
		{"positive even", 4, true},
		{"positive odd", 3, false},
		{"negative even", -2, true},
		{"negative odd", -1, false},
		{"one", 1, false},
		{"two", 2, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsEven(tt.n); got != tt.want {
				t.Errorf("IsEven(%d) = %v, want %v", tt.n, got, tt.want)
			}
		})
	}
}

func TestIsOdd(t *testing.T) {
	tests := []struct {
		name string
		n    int
		want bool
	}{
		{"zero", 0, false},
		{"positive odd", 3, true},
		{"positive even", 4, false},
		{"negative odd", -1, true},
		{"negative even", -2, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsOdd(tt.n); got != tt.want {
				t.Errorf("IsOdd(%d) = %v, want %v", tt.n, got, tt.want)
			}
		})
	}
}
