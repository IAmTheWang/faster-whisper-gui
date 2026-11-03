package transcribe

import (
	"strings"
	"testing"
)

func TestTailBufferKeepsOnlyTheEnd(t *testing.T) {
	b := newTailBuffer(10)
	for i := 0; i < 1000; i++ {
		b.Write([]byte("0123456789"))
	}
	b.Write([]byte("END"))
	got := b.String()
	if len(got) != 10 || !strings.HasSuffix(got, "END") {
		t.Fatalf("tail = %q, want last 10 bytes ending in END", got)
	}
}

func TestTailBufferShortWrites(t *testing.T) {
	b := newTailBuffer(100)
	b.Write([]byte("abc"))
	b.Write([]byte("def"))
	if got := b.String(); got != "abcdef" {
		t.Fatalf("tail = %q", got)
	}
}
