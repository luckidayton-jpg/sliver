package core

import (
	"sync"
	"testing"
)

// SeedJobID must raise the counter without ever lowering it, and concurrent
// NextJobID callers must never observe the same value (duplicate IDs collide
// on the listener_jobs.job_id unique index).
func TestSeedJobID(t *testing.T) {
	SeedJobID(2000)
	if got := NextJobID(); got != 2001 {
		t.Fatalf("NextJobID after SeedJobID(2000) = %d, want 2001", got)
	}
	SeedJobID(100) // lower seeds must not rewind
	if got := NextJobID(); got != 2002 {
		t.Fatalf("NextJobID after SeedJobID(100) = %d, want 2002", got)
	}
}

func TestNextJobIDConcurrentUnique(t *testing.T) {
	const n = 200
	seen := make(map[int]bool, n)
	var mu sync.Mutex
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id := NextJobID()
			mu.Lock()
			defer mu.Unlock()
			if seen[id] {
				t.Errorf("duplicate job id %d", id)
			}
			seen[id] = true
		}()
	}
	wg.Wait()
}
