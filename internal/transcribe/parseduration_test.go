package transcribe

import (
	"strings"
	"testing"
	"time"
)

func TestParseDuration(t *testing.T) {
	good := []struct {
		name string
		in   string
		want time.Duration
	}{
		{"fractional with newline", "12.5\n", 12*time.Second + 500*time.Millisecond},
		{"crlf terminator", "3.25\r\n", 3*time.Second + 250*time.Millisecond},
		{"surrounding spaces", "  7  ", 7 * time.Second},
		{"integer seconds", "120", 2 * time.Minute},
		{"sub-second", "0.5", 500 * time.Millisecond},
		{"exponent notation", "1.5e2", 150 * time.Second},
		{"plus sign", "+2", 2 * time.Second},
	}
	for _, tt := range good {
		t.Run("ok/"+tt.name, func(t *testing.T) {
			got, err := parseDuration(tt.in)
			if err != nil {
				t.Fatalf("parseDuration(%q) error: %v", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("parseDuration(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}

	bad := []struct {
		name string
		in   string
		msg  string // substring expected in the error
	}{
		{"empty", "", "parse ffprobe duration"},
		{"whitespace only", " \n", "parse ffprobe duration"},
		{"ffprobe N/A", "N/A\n", "parse ffprobe duration"},
		{"garbage", "abc", "parse ffprobe duration"},
		{"trailing junk", "12.5s", "parse ffprobe duration"},
		{"two values", "1.5\n2.5\n", "parse ffprobe duration"},
		{"NaN", "NaN", "unusable duration"},
		{"+Inf", "+Inf", "unusable duration"},
		{"-Inf", "-Inf", "unusable duration"},
		{"Infinity", "Infinity\n", "unusable duration"},
		{"zero", "0", "unusable duration"},
		{"zero float", "0.000000\n", "unusable duration"},
		{"negative", "-1.5", "unusable duration"},
	}
	for _, tt := range bad {
		t.Run("err/"+tt.name, func(t *testing.T) {
			got, err := parseDuration(tt.in)
			if err == nil {
				t.Fatalf("parseDuration(%q) = %v, want error", tt.in, got)
			}
			if got != 0 {
				t.Errorf("parseDuration(%q) returned %v alongside an error, want 0", tt.in, got)
			}
			if !strings.Contains(err.Error(), tt.msg) {
				t.Errorf("parseDuration(%q) error = %q, want substring %q", tt.in, err, tt.msg)
			}
		})
	}
}
