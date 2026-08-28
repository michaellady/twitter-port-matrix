package ids

import "testing"

func TestNextStartsAtOne(t *testing.T) {
	g := New()
	if got := g.Next(); got != 1 {
		t.Fatalf("Next()=%d, want 1", got)
	}
}

func TestNextIsStrictlyMonotonic(t *testing.T) {
	g := New()
	prev := g.Next()
	for i := 0; i < 100; i++ {
		next := g.Next()
		if next <= prev {
			t.Fatalf("not strictly monotonic: prev=%d next=%d", prev, next)
		}
		prev = next
	}
}

func TestPeekDoesNotAdvance(t *testing.T) {
	g := New()
	if got := g.Peek(); got != 1 {
		t.Fatalf("Peek()=%d, want 1", got)
	}
	if got := g.Peek(); got != 1 {
		t.Fatalf("Peek() advanced: got %d, want 1", got)
	}
	if got := g.Next(); got != 1 {
		t.Fatalf("Next after Peek=%d, want 1", got)
	}
	if got := g.Peek(); got != 2 {
		t.Fatalf("Peek after Next=%d, want 2", got)
	}
}

func TestNewAtClampsBelowOne(t *testing.T) {
	g := NewAt(0)
	if got := g.Next(); got != 1 {
		t.Fatalf("NewAt(0).Next()=%d, want 1 (clamped)", got)
	}
	g2 := NewAt(-5)
	if got := g2.Next(); got != 1 {
		t.Fatalf("NewAt(-5).Next()=%d, want 1 (clamped)", got)
	}
}

func TestNewAtUsesSeed(t *testing.T) {
	g := NewAt(100)
	if got := g.Next(); got != 100 {
		t.Fatalf("NewAt(100).Next()=%d, want 100", got)
	}
	if got := g.Next(); got != 101 {
		t.Fatalf("second Next()=%d, want 101", got)
	}
}

func TestCurrentBeforeAndAfterNext(t *testing.T) {
	g := New()
	if got := g.Current(); got != 0 {
		t.Fatalf("Current() before any Next: want 0, got %d", got)
	}
	g.Next()
	if got := g.Current(); got != 1 {
		t.Fatalf("Current() after first Next: want 1, got %d", got)
	}
	g.Next()
	g.Next()
	if got := g.Current(); got != 3 {
		t.Fatalf("Current() after three Nexts: want 3, got %d", got)
	}
}

func TestSetCurrentRoundTrip(t *testing.T) {
	g := New()
	g.SetCurrent(42)
	if got := g.Current(); got != 42 {
		t.Fatalf("Current after SetCurrent(42): want 42, got %d", got)
	}
	if got := g.Next(); got != 43 {
		t.Fatalf("Next after SetCurrent(42): want 43, got %d", got)
	}
}
