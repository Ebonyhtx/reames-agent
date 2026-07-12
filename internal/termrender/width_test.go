package termrender

import (
	"strings"
	"testing"
)

func TestStreamedRowsBasic(t *testing.T) {
	cases := []struct {
		name  string
		input string
		width int
		want  int
	}{
		{"no newline fits", "hello", 80, 0},
		{"newline only", "hello\n", 80, 1},
		{"two lines no trailing", "hello\nworld", 80, 1},
		{"two lines trailing", "hello\nworld\n", 80, 2},
		{"empty", "", 80, 0},
		{"single wrap", "abcdefghij", 5, 1},
		{"two wraps no nl", "abcdefghijklmno", 5, 2},
		{"line exactly width", strings.Repeat("a", 80), 80, 0},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if got := streamedRows(test.input, test.width); got != test.want {
				t.Errorf("streamedRows(%q, %d) = %d, want %d", test.input, test.width, got, test.want)
			}
		})
	}
}

func TestStreamedRowsCJK(t *testing.T) {
	input := strings.Repeat("中", 6)
	if got := streamedRows(input, 10); got != 1 {
		t.Errorf("streamedRows(6x中, 10) = %d, want 1", got)
	}
}

func TestStreamedRowsIgnoresANSI(t *testing.T) {
	input := "\x1b[1mhello\x1b[0m world"
	if got := streamedRows(input, 80); got != 0 {
		t.Errorf("ANSI line at width 80 = %d rows, want 0", got)
	}
}
