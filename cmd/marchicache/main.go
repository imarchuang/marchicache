package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/marchi/marchicache/internal/httpapi"
	"github.com/marchi/marchicache/internal/store"
)

func main() {
	addr := flag.String("addr", ":6380", "HTTP listen address")
	dataDir := flag.String("dataDir", "./data", "data directory for AOF")
	appendfsync := flag.String("appendfsync", "everysec", "AOF fsync: always|everysec|no")
	flag.Parse()

	policy, err := store.ParseFsync(*appendfsync)
	if err != nil {
		log.Fatal(err)
	}
	st, err := store.Open(*dataDir, policy)
	if err != nil {
		log.Fatal(err)
	}

	stop := make(chan struct{})
	st.StartActiveExpire(stop, 100*time.Millisecond)
	go func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
		<-ch
		close(stop)
		_ = st.Close()
		os.Exit(0)
	}()

	log.Printf("marchicache HTTP on %s dataDir=%s appendfsync=%s", *addr, *dataDir, policy)
	if err := http.ListenAndServe(*addr, httpapi.New(st)); err != nil {
		log.Fatal(err)
	}
}
