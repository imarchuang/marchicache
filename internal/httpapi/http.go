package httpapi

import (
	"encoding/json"
	"io"
	"net/http"

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
	s.st.Set(key, string(body))
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
