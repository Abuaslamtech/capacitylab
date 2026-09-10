package engine

import (
	"testing"
)

func TestParseMemoryMB(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
	}{
		{"512MB", 512},
		{"1GB", 1024},
		{"2GB", 2048},
		{"256mb", 256},
		{"1024", 1024},
	}

	for _, tc := range tests {
		got := ParseMemoryMB(tc.input)
		if got != tc.expected {
			t.Errorf("ParseMemoryMB(%q) = %d; expected %d", tc.input, got, tc.expected)
		}
	}
}
