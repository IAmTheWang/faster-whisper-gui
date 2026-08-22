package transcribe

import (
	"reflect"
	"testing"
)

func TestExtractAudioArgs(t *testing.T) {
	got := ExtractAudioArgs(`E:\videos\lecture1.mp4`, `E:\tmp\job1.wav`)
	want := []string{
		"-y",
		"-i", `E:\videos\lecture1.mp4`,
		"-vn",
		"-ar", "16000",
		"-ac", "1",
		"-c:a", "pcm_s16le",
		`E:\tmp\job1.wav`,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ExtractAudioArgs() = %#v, want %#v", got, want)
	}
}

func TestProbeDurationArgs(t *testing.T) {
	got := ProbeDurationArgs(`E:\videos\lecture1.mp4`)
	want := []string{
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		`E:\videos\lecture1.mp4`,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ProbeDurationArgs() = %#v, want %#v", got, want)
	}
}

func TestLastLines(t *testing.T) {
	tests := []struct {
		name string
		in   string
		n    int
		want string
	}{
		{"fewer than n", "a\nb", 5, "a\nb"},
		{"exactly n", "a\nb\nc", 3, "a\nb\nc"},
		{"more than n", "a\nb\nc\nd", 2, "c\nd"},
		{"trailing newline trimmed", "a\nb\n", 5, "a\nb"},
		{"empty", "", 5, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := lastLines(tt.in, tt.n); got != tt.want {
				t.Errorf("lastLines(%q, %d) = %q, want %q", tt.in, tt.n, got, tt.want)
			}
		})
	}
}
