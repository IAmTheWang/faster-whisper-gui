package job

import "testing"

func TestStoreAddGetList(t *testing.T) {
	s := NewStore()
	j1 := &Job{ID: "1", Status: StatusQueued}
	j2 := &Job{ID: "2", Status: StatusQueued}
	s.Add(j1)
	s.Add(j2)

	got, ok := s.Get("1")
	if !ok || got.ID != "1" {
		t.Fatalf("Get(1) = %+v, %v", got, ok)
	}

	if _, ok := s.Get("missing"); ok {
		t.Fatal("Get(missing) should return ok=false")
	}

	list := s.List()
	if len(list) != 2 {
		t.Fatalf("List() len = %d, want 2", len(list))
	}
	// newest first
	if list[0].ID != "2" || list[1].ID != "1" {
		t.Fatalf("List() order = [%s, %s], want [2, 1]", list[0].ID, list[1].ID)
	}
}

func TestStoreUpdateIsIsolatedFromSnapshots(t *testing.T) {
	s := NewStore()
	s.Add(&Job{ID: "1", Status: StatusQueued})

	snapshot, _ := s.Get("1")

	ok := s.Update("1", func(j *Job) { j.Status = StatusDone })
	if !ok {
		t.Fatal("Update(1) returned false")
	}

	if snapshot.Status != StatusQueued {
		t.Fatalf("earlier snapshot mutated: got %s, want %s (Get must return a copy)", snapshot.Status, StatusQueued)
	}

	got, _ := s.Get("1")
	if got.Status != StatusDone {
		t.Fatalf("Get(1) after Update = %s, want %s", got.Status, StatusDone)
	}

	if ok := s.Update("missing", func(j *Job) {}); ok {
		t.Fatal("Update(missing) should return false")
	}
}

func TestStoreCancel(t *testing.T) {
	s := NewStore()
	s.Add(&Job{ID: "1", Status: StatusQueued})

	if s.Cancel("1") {
		t.Fatal("Cancel should return false before a cancel func is set")
	}

	called := false
	s.SetCancel("1", func() { called = true })

	if !s.Cancel("1") {
		t.Fatal("Cancel should return true once a cancel func is set")
	}
	if !called {
		t.Fatal("Cancel did not invoke the stored cancel func")
	}

	if s.Cancel("missing") {
		t.Fatal("Cancel(missing) should return false")
	}

	s.Update("1", func(j *Job) { j.Status = StatusDone })
	called = false
	if s.Cancel("1") {
		t.Fatal("Cancel should return false once the job has reached a terminal status")
	}
	if called {
		t.Fatal("Cancel must not invoke the cancel func for an already-terminal job")
	}
}
