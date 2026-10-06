package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/marchi/marchicache/internal/httpapi"
	"github.com/marchi/marchicache/internal/loop"
	"github.com/marchi/marchicache/internal/store"
)

func main() {
	addr := flag.String("addr", ":6380", "HTTP listen address")
	redisAddr := flag.String("redisAddr", ":6379", "RESP listen address (empty to disable)")
	dataDir := flag.String("dataDir", "./data", "data directory for AOF")
	appendfsync := flag.String("appendfsync", "everysec", "AOF fsync: always|everysec|no")
	maxmemory := flag.Int64("maxmemory", 0, "max memory in bytes; 0 disables eviction")
	flag.Parse()

	policy, err := store.ParseFsync(*appendfsync)
	if err != nil {
		log.Fatal(err)
	}
	st, err := store.Open(*dataDir, policy)
	if err != nil {
		log.Fatal(err)
	}
	st.SetMaxMemory(*maxmemory)

	r, err := loop.New(st, *redisAddr)
	if err != nil {
		log.Fatal(err)
	}
	r.Start()

	go func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
		<-ch
		r.Stop()
		r.Do(func() { _ = st.Close() })
		os.Exit(0)
	}()

	log.Printf("marchicache HTTP %s RESP %s dataDir=%s appendfsync=%s", *addr, r.Addr, *dataDir, policy)
	if err := http.ListenAndServe(*addr, httpapi.New(st, r)); err != nil {
		log.Fatal(err)
	}
}
