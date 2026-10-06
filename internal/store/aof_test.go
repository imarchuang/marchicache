package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAOFReplaySETDEL(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir, FsyncAlways)
	if err != nil {
		t.Fatal(err)
	}
	s.Set("a", "1")
	s.Set("b", "2")
	s.Del("b")
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	s2, err := Open(dir, FsyncAlways)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s2.Close() })
	if v, ok, err := s2.Get("a"); err != nil || !ok || v != "1" {
		t.Fatalf("a=%q ok=%v err=%v", v, ok, err)
	}
	if _, ok, _ := s2.Get("b"); ok {
		t.Fatal("b should be gone")
	}
}

func TestAlwaysSurvivesKillWithoutFlush(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir, FsyncAlways)
	if err != nil {
		t.Fatal(err)
	}
	s.SetEX("session", "token-abc", 60*time.Second)
	if err := s.KillClose(); err != nil {
		t.Fatal(err)
	}

	s2, err := Open(dir, FsyncAlways)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s2.Close() })
	v, ok, err := s2.Get("session")
	if err != nil || !ok || v != "token-abc" {
		t.Fatalf("always should recover after kill: %q %v %v", v, ok, err)
	}
	if s2.TTL("session") <= 0 {
		t.Fatalf("remaining ttl %d", s2.TTL("session"))
	}
}

func TestEverysecNeedsCloseToGuarantee(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir, FsyncEverysec)
	if err != nil {
		t.Fatal(err)
	}
	s.Set("k", "v")
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s2, err := Open(dir, FsyncEverysec)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s2.Close() })
	if v, ok, err := s2.Get("k"); err != nil || !ok || v != "v" {
		t.Fatalf("everysec after close: %q %v %v", v, ok, err)
	}
}

func TestAOFFileGrows(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir, FsyncAlways)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	s.Set("k", "v")
	st, err := os.Stat(filepath.Join(dir, aofName))
	if err != nil {
		t.Fatal(err)
	}
	if st.Size() == 0 {
		t.Fatal("aof empty")
	}
	if s.AOFBytes() == 0 {
		t.Fatal("AOFBytes")
	}
	if s.FsyncPolicy() != "always" {
		t.Fatalf("policy %s", s.FsyncPolicy())
	}
}
