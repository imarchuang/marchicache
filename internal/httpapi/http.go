package httpapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/marchi/marchicache/internal/store"
)

type Server struct {
	st *store.Store
}

func New(st *store.Store) http.Handler {
	s := &Server{st: st}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.healthz)
	mux.HandleFunc("PUT /kv/{key}", s.putKV)
	mux.HandleFunc("GET /kv/{key}", s.getKV)
	mux.HandleFunc("DELETE /kv/{key}", s.delKV)
	mux.HandleFunc("POST /expire/{key}", s.expire)
	mux.HandleFunc("GET /ttl/{key}", s.ttl)
	return mux
}

func (s *Server) healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":    true,
		"keys":  s.st.Len(),
		"aof":   0,
		"fsync": "none",
	})
}

func (s *Server) putKV(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if ex := r.URL.Query().Get("ex"); ex != "" {
		sec, err := strconv.ParseInt(ex, 10, 64)
		if err != nil || sec <= 0 {
			http.Error(w, "invalid ex", http.StatusBadRequest)
			return
		}
		s.st.SetEX(key, string(body), time.Duration(sec)*time.Second)
	} else {
		s.st.Set(key, string(body))
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) getKV(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	v, ok := s.st.Get(key)
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(v))
}

func (s *Server) delKV(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if !s.st.Del(key) {
		http.NotFound(w, r)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) expire(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	ex := r.URL.Query().Get("ex")
	sec, err := strconv.ParseInt(ex, 10, 64)
	if err != nil || sec <= 0 {
		http.Error(w, "invalid ex", http.StatusBadRequest)
		return
	}
	if !s.st.Expire(key, time.Duration(sec)*time.Second) {
		http.NotFound(w, r)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) ttl(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(w, "%d", s.st.TTL(key))
}
