package engine

import (
	"testing"
)

func TestBuildLineIndex(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   []int
	}{
		{"empty", "", []int{0}},
		{"single line no newline", "hello", []int{0}},
		{"single line with newline", "hello\n", []int{0, 6}},
		{"multi line", "aaa\nbbb\nccc", []int{0, 4, 8}},
		{"trailing newline", "a\nb\n", []int{0, 2, 4}},
		{"CRLF", "a\r\nb\r\nc", []int{0, 3, 6}},
		{"empty lines", "\n\n\n", []int{0, 1, 2, 3}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildLineIndex([]byte(tt.source))
			if len(got) != len(tt.want) {
				t.Fatalf("len = %d, want %d: got %v", len(got), len(tt.want), got)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("index[%d] = %d, want %d", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestOffsetToLineCol(t *testing.T) {
	// Source: "aaa\nbbb\nccc"
	// Line 1: bytes 0-3 (aaa\n)
	// Line 2: bytes 4-7 (bbb\n)
	// Line 3: bytes 8-10 (ccc)
	lineStarts := []int{0, 4, 8}

	tests := []struct {
		name     string
		offset   int
		wantLine int
		wantCol  int
	}{
		{"start of file", 0, 1, 0},
		{"mid first line", 2, 1, 2},
		{"start of second line", 4, 2, 0},
		{"mid second line", 5, 2, 1},
		{"start of third line", 8, 3, 0},
		{"end of file", 10, 3, 2},
		{"at newline boundary", 3, 1, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			line, col := offsetToLineCol(lineStarts, tt.offset)
			if line != tt.wantLine || col != tt.wantCol {
				t.Errorf("offset %d → (%d, %d), want (%d, %d)", tt.offset, line, col, tt.wantLine, tt.wantCol)
			}
		})
	}
}
