package httpapi

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/marchi/marchicache/internal/store"
)

func TestKVAndHealthz(t *testing.T) {
	h := New(store.New())
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	put, err := http.NewRequest(http.MethodPut, srv.URL+"/kv/session", strings.NewReader("token-abc"))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(put)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("PUT status %d", resp.StatusCode)
	}

	resp, err = http.Get(srv.URL + "/kv/session")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || string(body) != "token-abc" {
		t.Fatalf("GET %d %q", resp.StatusCode, body)
	}

	resp, err = http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	hb, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("healthz %d", resp.StatusCode)
	}
	if !strings.Contains(string(hb), `"keys":1`) {
		t.Fatalf("healthz body %s", hb)
	}

	del, err := http.NewRequest(http.MethodDelete, srv.URL+"/kv/session", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err = http.DefaultClient.Do(del)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("DEL status %d", resp.StatusCode)
	}

	resp, err = http.Get(srv.URL + "/kv/session")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET after del %d", resp.StatusCode)
	}
}

func TestEXPIREAndTTL(t *testing.T) {
	h := New(store.New())
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	put, err := http.NewRequest(http.MethodPut, srv.URL+"/kv/a?ex=60", strings.NewReader("v"))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(put)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("PUT ex %d", resp.StatusCode)
	}

	resp, err = http.Get(srv.URL + "/ttl/a")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || string(body) == "-2" || string(body) == "-1" {
		t.Fatalf("ttl %d %q", resp.StatusCode, body)
	}

	resp, err = http.Post(srv.URL+"/expire/a?ex=30", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expire %d", resp.StatusCode)
	}

	resp, err = http.Get(srv.URL + "/ttl/missing")
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(body) != "-2" {
		t.Fatalf("missing ttl %q", body)
	}
}
