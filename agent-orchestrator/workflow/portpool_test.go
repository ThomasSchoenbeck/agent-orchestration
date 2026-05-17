package workflow

import (
	"testing"
)

func TestPortPool_AcquireRelease(t *testing.T) {
	p := NewPortPool(18000, 3) // pool: 18000, 18001, 18002

	a, err := p.Acquire()
	if err != nil {
		t.Fatalf("first Acquire: %v", err)
	}
	if a != 18000 {
		t.Errorf("got port %d, want 18000", a)
	}

	b, err := p.Acquire()
	if err != nil {
		t.Fatalf("second Acquire: %v", err)
	}
	if b != 18001 {
		t.Errorf("got port %d, want 18001", b)
	}

	// Release a; re-acquiring should return a valid free port (not necessarily a).
	p.Release(a)
	c, err := p.Acquire()
	if err != nil {
		t.Fatalf("third Acquire after release: %v", err)
	}
	if c == b {
		t.Errorf("got port %d which is still held by b", c)
	}
	if c < 18000 || c > 18002 {
		t.Errorf("got port %d outside pool range [18000, 18002]", c)
	}
}

func TestPortPool_Exhausted(t *testing.T) {
	p := NewPortPool(19000, 2)
	_, _ = p.Acquire()
	_, _ = p.Acquire()
	_, err := p.Acquire()
	if err == nil {
		t.Fatal("expected error on exhausted pool, got nil")
	}
}

func TestPortPool_ReleaseZeroNoOp(t *testing.T) {
	p := NewPortPool(20000, 1)
	// Should not panic or affect the pool.
	p.Release(0)
	port, err := p.Acquire()
	if err != nil {
		t.Fatalf("Acquire after Release(0): %v", err)
	}
	if port != 20000 {
		t.Errorf("got %d, want 20000", port)
	}
}

func TestPortPool_DistinctPorts(t *testing.T) {
	p := NewPortPool(30000, 10)
	seen := map[int]bool{}
	for i := 0; i < 10; i++ {
		port, err := p.Acquire()
		if err != nil {
			t.Fatalf("Acquire #%d: %v", i, err)
		}
		if seen[port] {
			t.Errorf("duplicate port %d", port)
		}
		seen[port] = true
	}
}
