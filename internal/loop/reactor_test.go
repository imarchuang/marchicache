//go:build darwin || linux

package loop

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/marchi/marchicache/internal/store"
)

func encode(args ...string) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "*%d\r\n", len(args))
	for _, a := range args {
		fmt.Fprintf(&b, "$%d\r\n%s\r\n", len(a), a)
	}
	return []byte(b.String())
}

func startR(t *testing.T) *Reactor {
	t.Helper()
	st := store.New()
	r, err := New(st, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	r.Start()
	t.Cleanup(r.Stop)
	return r
}

func TestRESPPingSetGetPipeline(t *testing.T) {
	r := startR(t)
	c, err := net.Dial("tcp", r.Addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	_, err = c.Write(append(encode("PING"), encode("SET", "a", "1")...))
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Write(encode("GET", "a"))
	if err != nil {
		t.Fatal(err)
	}
	br := bufio.NewReader(c)
	if line, _ := br.ReadString('\n'); strings.TrimSpace(line) != "+PONG" {
		t.Fatalf("ping %q", line)
	}
	if line, _ := br.ReadString('\n'); strings.TrimSpace(line) != "+OK" {
		t.Fatalf("set %q", line)
	}
	if _, err := br.ReadString('\n'); err != nil {
		t.Fatal(err)
	}
	val, err := br.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(val) != "1" {
		t.Fatalf("get %q", val)
	}
}

func TestManyConnsRace(t *testing.T) {
	r := startR(t)
	const n = 100
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		i := i
		go func() {
			defer wg.Done()
			c, err := net.DialTimeout("tcp", r.Addr, 2*time.Second)
			if err != nil {
				t.Errorf("dial %v", err)
				return
			}
			defer c.Close()
			key := "k" + strconv.Itoa(i)
			msg := append(encode("SET", key, "v"), encode("GET", key)...)
			if _, err := c.Write(msg); err != nil {
				t.Errorf("write %v", err)
				return
			}
			br := bufio.NewReader(c)
			if line, _ := br.ReadString('\n'); strings.TrimSpace(line) != "+OK" {
				t.Errorf("set %q", line)
				return
			}
			if _, err := br.ReadString('\n'); err != nil {
				t.Errorf("bulk hdr %v", err)
				return
			}
			val, err := br.ReadString('\n')
			if err != nil || strings.TrimSpace(val) != "v" {
				t.Errorf("get %q %v", val, err)
			}
		}()
	}
	wg.Wait()
}

func TestExpireAndHTTPDo(t *testing.T) {
	r := startR(t)
	c, err := net.Dial("tcp", r.Addr)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_, _ = c.Write(encode("SET", "x", "y", "EX", "60"))
	br := bufio.NewReader(c)
	line, _ := br.ReadString('\n')
	if strings.TrimSpace(line) != "+OK" {
		t.Fatalf("%q", line)
	}
	_, _ = c.Write(encode("EXPIRE", "x", "30"))
	line, _ = br.ReadString('\n')
	if strings.TrimSpace(line) != ":1" {
		t.Fatalf("expire %q", line)
	}
	var ttl int64
	r.Do(func() {
		ttl = r.st.TTL("x")
	})
	if ttl <= 0 {
		t.Fatalf("ttl %d", ttl)
	}
}

func TestInlinePING(t *testing.T) {
	r := startR(t)
	c, err := net.Dial("tcp", r.Addr)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_, _ = c.Write([]byte("PING\r\n"))
	b, err := io.ReadAll(io.LimitReader(c, 7))
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "+PONG\r\n" {
		t.Fatalf("%q", b)
	}
}
