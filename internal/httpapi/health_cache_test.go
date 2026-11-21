package httpapi

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"faster-whisper-gui/internal/transcribe"
)

func okHealth() healthResponse {
	return healthResponse{Ffmpeg: transcribe.ComponentHealth{OK: true}, WhisperCli: transcribe.ComponentHealth{OK: true}}
}

func TestHealthCacheCoalescesConcurrentProbes(t *testing.T) {
	var c healthCache
	var probes int32
	release := make(chan struct{})
	probe := func() healthResponse {
		atomic.AddInt32(&probes, 1)
		<-release
		return okHealth()
	}

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); c.do("f", "w", probe) }()
	}
	time.Sleep(50 * time.Millisecond) // let every caller reach do()
	close(release)
	wg.Wait()
	if n := atomic.LoadInt32(&probes); n != 1 {
		t.Fatalf("probe ran %d times, want 1", n)
	}
	c.do("f", "w", probe) // served from cache
	if n := atomic.LoadInt32(&probes); n != 1 {
		t.Fatalf("cached call re-probed (%d)", n)
	}
}

func TestHealthCacheKeyedOnPaths(t *testing.T) {
	var c healthCache
	var probes int
	probe := func() healthResponse { probes++; return okHealth() }
	c.do("f", "w", probe)
	c.do("f2", "w", probe)
	if probes != 2 {
		t.Fatalf("probes = %d, want 2 (different ffmpeg path)", probes)
	}
}

func TestHealthCacheFailureExpiresQuickly(t *testing.T) {
	var c healthCache
	var probes int
	probe := func() healthResponse { probes++; return healthResponse{} } // both not OK
	c.do("f", "w", probe)
	c.do("f", "w", probe)
	if probes != 1 {
		t.Fatalf("failure should be cached briefly, probes = %d", probes)
	}
	c.at = c.at.Add(-healthFailureTTL - time.Second)
	c.do("f", "w", probe)
	if probes != 2 {
		t.Fatalf("failure cache should expire after healthFailureTTL, probes = %d", probes)
	}
}
