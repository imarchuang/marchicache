package store

import "testing"

func TestSetGetDel(t *testing.T) {
	s := New()
	if _, ok := s.Get("a"); ok {
		t.Fatal("expected miss")
	}
	s.Set("a", "1")
	v, ok := s.Get("a")
	if !ok || v != "1" {
		t.Fatalf("got %q ok=%v", v, ok)
	}
	if !s.Del("a") {
		t.Fatal("expected del")
	}
	if _, ok := s.Get("a"); ok {
		t.Fatal("expected miss after del")
	}
	if s.Del("a") {
		t.Fatal("second del should miss")
	}
	if s.Len() != 0 {
		t.Fatalf("len=%d", s.Len())
	}
}
