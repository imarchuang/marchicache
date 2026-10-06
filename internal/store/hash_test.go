package store

import (
	"testing"
	"time"
)

func TestHashHSetHGetAll(t *testing.T) {
	s := New()
	n, err := s.HSet("u", "name", "marc")
	if err != nil || n != 1 {
		t.Fatalf("hset %d %v", n, err)
	}
	v, ok, err := s.HGet("u", "name")
	if err != nil || !ok || v != "marc" {
		t.Fatalf("hget %q %v %v", v, ok, err)
	}
	all, err := s.HGetAll("u")
	if err != nil || all["name"] != "marc" {
		t.Fatalf("hgetall %v %v", all, err)
	}
}

func TestWrongTypeSETThenHGET(t *testing.T) {
	s := New()
	s.Set("a", "1")
	_, _, err := s.HGet("a", "f")
	if err != ErrWrongType {
		t.Fatalf("want wrongtype, got %v", err)
	}
	_, err = s.HSet("a", "f", "v")
	if err != ErrWrongType {
		t.Fatalf("hset want wrongtype, got %v", err)
	}
	_, _, err = s.Get("a")
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.HGetAll("a")
	if err != ErrWrongType {
		t.Fatalf("hgetall %v", err)
	}
}

func TestZSetRange(t *testing.T) {
	s := New()
	_, err := s.ZAdd("lb", "alice", 10)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = s.ZAdd("lb", "bob", 5)
	_, _ = s.ZAdd("lb", "cara", 7)
	got, err := s.ZRange("lb", 0, -1)
	if err != nil || len(got) != 3 {
		t.Fatalf("%v %v", got, err)
	}
	if got[0].Member != "bob" || got[2].Member != "alice" {
		t.Fatalf("order %+v", got)
	}
	s.Set("s", "x")
	if _, err := s.ZAdd("s", "m", 1); err != ErrWrongType {
		t.Fatalf("zadd wrongtype %v", err)
	}
}

func TestHashAOFReplay(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir, FsyncAlways)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.HSet("h", "f", "v"); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s2, err := Open(dir, FsyncAlways)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s2.Close() })
	v, ok, err := s2.HGet("h", "f")
	if err != nil || !ok || v != "v" {
		t.Fatalf("replay hget %q %v %v", v, ok, err)
	}
}

func TestExpireStillWorksOnHash(t *testing.T) {
	clk := &fakeClock{now: time.Unix(0, 0)}
	s := NewWithClock(clk)
	_, _ = s.HSet("h", "f", "v")
	s.Expire("h", time.Second)
	clk.now = clk.now.Add(2 * time.Second)
	all, err := s.HGetAll("h")
	if err != nil || len(all) != 0 {
		t.Fatalf("%v %v", all, err)
	}
}
