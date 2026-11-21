package sseutil

import (
	"fmt"
	"sync"
	"testing"
)

func TestTerminalEventEvictsOldestWhenBufferFull(t *testing.T) {
	b := NewBroker()
	ch, unsub := b.Subscribe("j")
	defer unsub()

	for i := 0; i < cap(ch)+5; i++ {
		b.Publish("j", "progress", i)
	}
	if len(ch) != cap(ch) {
		t.Fatalf("buffer should be full, len=%d cap=%d", len(ch), cap(ch))
	}
	b.Publish("j", "done", nil)

	var last Event
	for len(ch) > 0 {
		last = <-ch
	}
	if last.Type != "done" {
		t.Fatalf("terminal event was dropped, last event = %q", last.Type)
	}
}

func TestProgressEventDroppedWhenBufferFull(t *testing.T) {
	b := NewBroker()
	ch, unsub := b.Subscribe("j")
	defer unsub()
	for i := 0; i < cap(ch)+3; i++ {
		b.Publish("j", "progress", i)
	}
	first := <-ch
	if first.Data != 0 {
		t.Fatalf("progress overflow must drop the newest, first = %v", first.Data)
	}
}

func TestUnsubscribeIsIdempotent(t *testing.T) {
	b := NewBroker()
	_, unsub := b.Subscribe("j")
	unsub()
	unsub() // must not panic on a double close
}

func TestConcurrentPublishAndUnsubscribe(t *testing.T) {
	b := NewBroker()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(2)
		ch, unsub := b.Subscribe("j")
		go func() {
			defer wg.Done()
			for range ch {
			}
		}()
		go func(n int) {
			defer wg.Done()
			b.Publish("j", "progress", fmt.Sprint(n))
			unsub()
		}(i)
	}
	wg.Wait()
}
