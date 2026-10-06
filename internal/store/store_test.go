package store

import (
	"testing"
	"time"
)

type fakeClock struct {
	now time.Time
}

func (f *fakeClock) Now() time.Time { return f.now }

func TestSetGetDel(t *testing.T) {
	s := New()
	if _, ok, _ := s.Get("a"); ok {
		t.Fatal("expected miss")
	}
	s.Set("a", "1")
	v, ok, err := s.Get("a")
	if err != nil || !ok || v != "1" {
		t.Fatalf("got %q ok=%v err=%v", v, ok, err)
	}
	if !s.Del("a") {
		t.Fatal("expected del")
	}
	if _, ok, _ := s.Get("a"); ok {
		t.Fatal("expected miss after del")
	}
	if s.Del("a") {
		t.Fatal("second del should miss")
	}
	if s.Len() != 0 {
		t.Fatalf("len=%d", s.Len())
	}
}

func TestLazyExpireWithFakeClock(t *testing.T) {
	clk := &fakeClock{now: time.Unix(1000, 0)}
	s := NewWithClock(clk)
	s.SetEX("k", "v", 10*time.Second)
	if s.TTL("k") != 10 {
		t.Fatalf("ttl=%d", s.TTL("k"))
	}
	clk.now = clk.now.Add(9 * time.Second)
	if v, ok, err := s.Get("k"); err != nil || !ok || v != "v" {
		t.Fatalf("still live: %q %v %v", v, ok, err)
	}
	clk.now = clk.now.Add(2 * time.Second)
	if _, ok, _ := s.Get("k"); ok {
		t.Fatal("lazy expire should miss after T")
	}
	if s.TTL("k") != TTLMissing {
		t.Fatalf("ttl after expire %d", s.TTL("k"))
	}
}

func TestExpireCommand(t *testing.T) {
	clk := &fakeClock{now: time.Unix(0, 0)}
	s := NewWithClock(clk)
	if s.Expire("missing", time.Second) {
		t.Fatal("expire missing")
	}
	s.Set("a", "1")
	if s.TTL("a") != TTLNoExpire {
		t.Fatalf("no expire ttl=%d", s.TTL("a"))
	}
	if !s.Expire("a", 5*time.Second) {
		t.Fatal("expire")
	}
	if s.TTL("a") != 5 {
		t.Fatalf("ttl=%d", s.TTL("a"))
	}
	clk.now = clk.now.Add(5 * time.Second)
	if s.Expire("a", time.Second) {
		t.Fatal("already expired")
	}
}

func TestActiveExpire(t *testing.T) {
	clk := &fakeClock{now: time.Unix(0, 0)}
	s := NewWithClock(clk)
	s.SetEX("hot", "1", time.Hour)
	s.SetEX("cold", "2", time.Second)
	clk.now = clk.now.Add(2 * time.Second)
	n := s.ActiveExpire(20)
	if n != 1 {
		t.Fatalf("expired %d", n)
	}
	if _, ok, _ := s.Get("cold"); ok {
		t.Fatal("cold should be gone")
	}
	if _, ok, _ := s.Get("hot"); !ok {
		t.Fatal("hot should remain")
	}
}
