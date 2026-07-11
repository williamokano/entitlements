package id

import (
	"sync"
	"testing"

	"github.com/google/uuid"
)

func TestUUIDv7UniqueAndOrdered(t *testing.T) {
	var g UUIDv7
	const n = 1000

	seen := make(map[uuid.UUID]struct{}, n)
	prev := g.New()
	seen[prev] = struct{}{}

	for i := 1; i < n; i++ {
		cur := g.New()
		if _, dup := seen[cur]; dup {
			t.Fatalf("duplicate UUID generated at %d: %s", i, cur)
		}
		seen[cur] = struct{}{}

		if cur.Version() != 7 {
			t.Fatalf("UUID version = %d, want 7", cur.Version())
		}
		// UUIDv7 is time-ordered; byte-wise comparison must be non-decreasing.
		if cur.String() < prev.String() {
			t.Fatalf("UUIDv7 not ordered: %s < %s", cur, prev)
		}
		prev = cur
	}
}

func TestSequenceDeterministic(t *testing.T) {
	a := NewSequence()
	b := NewSequence()
	for i := 0; i < 5; i++ {
		if a.New() != b.New() {
			t.Fatalf("two fresh sequences diverged at %d", i)
		}
	}

	// The nth value (1-indexed) encodes n in the low 64 bits.
	s := NewSequence()
	_ = s.New() // 1
	_ = s.New() // 2
	third := s.New()
	want := uuid.UUID{}
	want[15] = 3
	if third != want {
		t.Fatalf("third value = %s, want %s", third, want)
	}
}

func TestSequenceConcurrentUnique(t *testing.T) {
	s := NewSequence()
	const goroutines, per = 8, 100

	var wg sync.WaitGroup
	var mu sync.Mutex
	seen := make(map[uuid.UUID]struct{}, goroutines*per)

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < per; i++ {
				u := s.New()
				mu.Lock()
				seen[u] = struct{}{}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if len(seen) != goroutines*per {
		t.Fatalf("got %d unique ids, want %d", len(seen), goroutines*per)
	}
}
