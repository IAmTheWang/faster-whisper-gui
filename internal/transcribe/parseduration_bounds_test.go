package transcribe

import "testing"

func TestParseDurationRejectsUnusableMagnitudes(t *testing.T) {
	for _, in := range []string{"1e-12", "0.0000000001", "1e12", "99999999999"} {
		if d, err := parseDuration(in); err == nil {
			t.Errorf("parseDuration(%q) = %v, want an error", in, d)
		}
	}
	if d, err := parseDuration("12.5\n"); err != nil || d.Seconds() != 12.5 {
		t.Errorf("parseDuration(12.5) = %v, %v", d, err)
	}
}
