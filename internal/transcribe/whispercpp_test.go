package transcribe

import (
	"reflect"
	"testing"
	"time"
)

func TestBuildWhisperArgs(t *testing.T) {
	tests := []struct {
		name string
		opts Options
		want []string
	}{
		{
			name: "basic, language defaults untouched",
			opts: Options{
				ModelPath:    `E:\models\ggml-small.bin`,
				AudioPath:    `E:\tmp\job1.wav`,
				Language:     "ja",
				OutputPrefix: `E:\videos\lecture1`,
			},
			want: []string{
				"-m", `E:\models\ggml-small.bin`,
				"-f", `E:\tmp\job1.wav`,
				"-l", "ja",
				"-of", `E:\videos\lecture1`,
				"-osrt",
			},
		},
		{
			name: "empty language defaults to auto",
			opts: Options{
				ModelPath:    `E:\models\ggml-small.bin`,
				AudioPath:    `E:\tmp\job1.wav`,
				Language:     "",
				OutputPrefix: `E:\videos\lecture1`,
			},
			want: []string{
				"-m", `E:\models\ggml-small.bin`,
				"-f", `E:\tmp\job1.wav`,
				"-l", "auto",
				"-of", `E:\videos\lecture1`,
				"-osrt",
			},
		},
		{
			name: "maxLen appends -ml",
			opts: Options{
				ModelPath:    `E:\models\ggml-small.bin`,
				AudioPath:    `E:\tmp\job1.wav`,
				Language:     "auto",
				OutputPrefix: `E:\videos\lecture1`,
				MaxLen:       16,
			},
			want: []string{
				"-m", `E:\models\ggml-small.bin`,
				"-f", `E:\tmp\job1.wav`,
				"-l", "auto",
				"-of", `E:\videos\lecture1`,
				"-osrt",
				"-ml", "16",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildWhisperArgs(tt.opts)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("BuildWhisperArgs() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestParseSegmentEnd(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		wantEnd  time.Duration
		wantText string
		wantOK   bool
	}{
		{
			name:     "standard segment line",
			line:     "[00:00:12.340 --> 00:00:15.560]   Hello there.",
			wantEnd:  15*time.Second + 560*time.Millisecond,
			wantText: "Hello there.",
			wantOK:   true,
		},
		{
			name:     "with hours",
			line:     "[01:02:03.000 --> 01:02:05.500] text",
			wantEnd:  1*time.Hour + 2*time.Minute + 5*time.Second + 500*time.Millisecond,
			wantText: "text",
			wantOK:   true,
		},
		{
			name:   "not a segment line",
			line:   "whisper_init_from_file: loading model",
			wantOK: false,
		},
		{
			name:   "progress line should not match",
			line:   "whisper_print_progress_callback: progress =  42%",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			end, text, ok := parseSegmentEnd(tt.line)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if end != tt.wantEnd {
				t.Errorf("end = %v, want %v", end, tt.wantEnd)
			}
			if text != tt.wantText {
				t.Errorf("text = %q, want %q", text, tt.wantText)
			}
		})
	}
}

func TestParseStderrPercent(t *testing.T) {
	tests := []struct {
		name   string
		line   string
		want   float64
		wantOK bool
	}{
		{"typical", "whisper_print_progress_callback: progress =  42%", 42, true},
		{"no spaces", "progress=7%", 7, true},
		{"unrelated line", "whisper_init_from_file: loading model", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseStderrPercent(tt.line)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}
