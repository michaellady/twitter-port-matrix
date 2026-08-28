package clock

import "testing"

func TestNewStartsAtZero(t *testing.T) {
	c := New()
	if got := c.Now(); got != 0 {
		t.Fatalf("Now()=%d, want 0", got)
	}
}

func TestNewAtStartsAtGiven(t *testing.T) {
	c := NewAt(42)
	if got := c.Now(); got != 42 {
		t.Fatalf("Now()=%d, want 42", got)
	}
}

func TestNowDoesNotAdvance(t *testing.T) {
	c := New()
	a := c.Now()
	b := c.Now()
	if a != b {
		t.Fatalf("Now() advanced without Tick: a=%d b=%d", a, b)
	}
}

func TestTickAdvancesByOne(t *testing.T) {
	c := New()
	before := c.Now()
	c.Tick()
	after := c.Now()
	if after != before+1 {
		t.Fatalf("Tick: before=%d after=%d, want before+1", before, after)
	}
}

func TestTickIsMonotonic(t *testing.T) {
	c := New()
	prev := c.Now()
	for i := 0; i < 100; i++ {
		c.Tick()
		now := c.Now()
		if now < prev {
			t.Fatalf("clock went backwards: prev=%d now=%d", prev, now)
		}
		prev = now
	}
}

func TestSetNowAdvancesForward(t *testing.T) {
	c := New()
	c.SetNow(100)
	if got := c.Now(); got != 100 {
		t.Fatalf("after SetNow(100): want 100, got %d", got)
	}
	c.SetNow(150)
	if got := c.Now(); got != 150 {
		t.Fatalf("after SetNow(150): want 150, got %d", got)
	}
}

func TestSetNowIgnoresBackwards(t *testing.T) {
	c := NewAt(50)
	c.SetNow(10)
	if got := c.Now(); got != 50 {
		t.Fatalf("backwards SetNow should be ignored: want 50, got %d", got)
	}
}

func TestSetNowEqualIsNoop(t *testing.T) {
	c := NewAt(50)
	c.SetNow(50)
	if got := c.Now(); got != 50 {
		t.Fatalf("equal SetNow: want 50, got %d", got)
	}
}
