package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRewriteShrinksAOFAndKeepsKeys(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir, FsyncAlways)
	if err != nil {
		t.Fatal(err)
	}
	s.Set("keep", "v")
	for i := 0; i < 50; i++ {
		s.Set("tmp", "x")
		s.Del("tmp")
	}
	before, err := os.Stat(filepath.Join(dir, aofName))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Rewrite(); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(filepath.Join(dir, aofName))
	if err != nil {
		t.Fatal(err)
	}
	if after.Size() >= before.Size() {
		t.Fatalf("rewrite did not shrink %d -> %d", before.Size(), after.Size())
	}
	if v, ok, err := s.Get("keep"); err != nil || !ok || v != "v" {
		t.Fatalf("live get after rewrite %q %v %v", v, ok, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s2, err := Open(dir, FsyncAlways)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s2.Close() })
	if v, ok, err := s2.Get("keep"); err != nil || !ok || v != "v" {
		t.Fatalf("replay after rewrite %q %v %v", v, ok, err)
	}
}

func TestApproxLRUEvictsCold(t *testing.T) {
	clk := &fakeClock{now: time.Unix(0, 0)}
	s := NewWithClock(clk)
	s.SetMaxMemory(250)
	clk.now = time.Unix(1, 0)
	s.Set("cold", strings.Repeat("c", 80))
	clk.now = time.Unix(2, 0)
	s.Set("hot", "h")
	clk.now = time.Unix(3, 0)
	if _, ok, err := s.Get("hot"); err != nil || !ok {
		t.Fatal("hot")
	}
	clk.now = time.Unix(4, 0)
	s.Set("new", strings.Repeat("n", 80))
	if _, ok, _ := s.Get("hot"); !ok {
		t.Fatal("hot should survive")
	}
	if _, ok, _ := s.Get("cold"); ok {
		t.Fatal("cold should be evicted")
	}
}
