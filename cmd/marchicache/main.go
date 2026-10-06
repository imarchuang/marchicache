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
	dataDir := flag.String("dataDir", "./data", "data directory (unused until AOF)")
	flag.Parse()
	_ = dataDir

	st := store.New()
	stop := make(chan struct{})
	st.StartActiveExpire(stop, 100*time.Millisecond)
	go func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
		<-ch
		close(stop)
		os.Exit(0)
	}()

	log.Printf("marchicache HTTP on %s", *addr)
	if err := http.ListenAndServe(*addr, httpapi.New(st)); err != nil {
		log.Fatal(err)
	}
}
