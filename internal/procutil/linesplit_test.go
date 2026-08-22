package procutil

import (
	"bufio"
	"strings"
	"testing"
)

func scanAll(t *testing.T, input string) []string {
	t.Helper()
	s := bufio.NewScanner(strings.NewReader(input))
	s.Split(ScanCRLines)
	var got []string
	for s.Scan() {
		got = append(got, s.Text())
	}
	if err := s.Err(); err != nil {
		t.Fatalf("scanner error: %v", err)
	}
	return got
}

func TestScanCRLines(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "plain newlines",
			input: "line1\nline2\nline3",
			want:  []string{"line1", "line2", "line3"},
		},
		{
			name:  "crlf newlines",
			input: "line1\r\nline2\r\nline3\r\n",
			want:  []string{"line1", "line2", "line3"},
		},
		{
			name:  "bare cr progress redraws",
			input: "progress = 1%\rprogress = 2%\rprogress = 100%\n",
			want:  []string{"progress = 1%", "progress = 2%", "progress = 100%"},
		},
		{
			name:  "mixed cr and lf",
			input: "progress = 1%\rprogress = 50%\r[00:00:01.000 --> 00:00:02.000] hello\ndone\r\n",
			want:  []string{"progress = 1%", "progress = 50%", "[00:00:01.000 --> 00:00:02.000] hello", "done"},
		},
		{
			name:  "no trailing terminator",
			input: "line1\nline2",
			want:  []string{"line1", "line2"},
		},
		{
			name:  "trailing bare cr at eof",
			input: "line1\rline2\r",
			want:  []string{"line1", "line2"},
		},
		{
			name:  "empty input",
			input: "",
			want:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := scanAll(t, tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d lines %q, want %d lines %q", len(got), got, len(tt.want), tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("line %d: got %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}
