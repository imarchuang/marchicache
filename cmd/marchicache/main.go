package main

import (
	"flag"
	"log"
	"net/http"

	"github.com/marchi/marchicache/internal/httpapi"
	"github.com/marchi/marchicache/internal/store"
)

func main() {
	addr := flag.String("addr", ":6380", "HTTP listen address")
	dataDir := flag.String("dataDir", "./data", "data directory (unused in slice 0)")
	flag.Parse()
	_ = dataDir

	st := store.New()
	log.Printf("marchicache HTTP on %s", *addr)
	if err := http.ListenAndServe(*addr, httpapi.New(st)); err != nil {
		log.Fatal(err)
	}
}
