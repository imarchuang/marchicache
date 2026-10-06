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
	mux.HandleFunc("PUT /hash/{key}/{field}", s.putHash)
	mux.HandleFunc("GET /hash/{key}/{field}", s.getHashField)
	mux.HandleFunc("GET /hash/{key}", s.getHash)
	mux.HandleFunc("PUT /zset/{key}/{member}", s.putZSet)
	mux.HandleFunc("GET /zset/{key}", s.getZSet)
	return mux
}

func (s *Server) healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":    true,
		"keys":  s.st.Len(),
		"aof":   s.st.AOFBytes(),
		"fsync": s.st.FsyncPolicy(),
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
	v, ok, err := s.st.Get(key)
	if err == store.ErrWrongType {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
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

func (s *Server) putHash(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	field := r.PathValue("field")
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if _, err := s.st.HSet(key, field, string(body)); err != nil {
		if err == store.ErrWrongType {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) getHashField(w http.ResponseWriter, r *http.Request) {
	v, ok, err := s.st.HGet(r.PathValue("key"), r.PathValue("field"))
	if err == store.ErrWrongType {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(v))
}

func (s *Server) getHash(w http.ResponseWriter, r *http.Request) {
	all, err := s.st.HGetAll(r.PathValue("key"))
	if err == store.ErrWrongType {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(all)
}

func (s *Server) putZSet(w http.ResponseWriter, r *http.Request) {
	score, err := strconv.ParseFloat(r.URL.Query().Get("score"), 64)
	if err != nil {
		http.Error(w, "invalid score", http.StatusBadRequest)
		return
	}
	if _, err := s.st.ZAdd(r.PathValue("key"), r.PathValue("member"), score); err != nil {
		if err == store.ErrWrongType {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) getZSet(w http.ResponseWriter, r *http.Request) {
	got, err := s.st.ZRange(r.PathValue("key"), 0, -1)
	if err == store.ErrWrongType {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	type row struct {
		Member string  `json:"member"`
		Score  float64 `json:"score"`
	}
	out := make([]row, 0, len(got))
	for _, z := range got {
		out = append(out, row{Member: z.Member, Score: z.Score})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}
