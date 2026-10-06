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

type Executor interface {
	Do(fn func())
}

type serial struct {
	ch chan func()
}

func Serial() Executor {
	s := &serial{ch: make(chan func(), 64)}
	go func() {
		for fn := range s.ch {
			fn()
		}
	}()
	return s
}

func (s *serial) Do(fn func()) {
	done := make(chan struct{})
	s.ch <- func() {
		defer close(done)
		fn()
	}
	<-done
}

type Server struct {
	st   *store.Store
	exec Executor
}

func New(st *store.Store, exec Executor) http.Handler {
	s := &Server{st: st, exec: exec}
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
	mux.HandleFunc("POST /rewrite", s.rewrite)
	return mux
}

func (s *Server) healthz(w http.ResponseWriter, _ *http.Request) {
	var keys int
	var aof int64
	var fsync string
	s.exec.Do(func() {
		keys = s.st.Len()
		aof = s.st.AOFBytes()
		fsync = s.st.FsyncPolicy()
	})
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":    true,
		"keys":  keys,
		"aof":   aof,
		"fsync": fsync,
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
		s.exec.Do(func() { s.st.SetEX(key, string(body), time.Duration(sec)*time.Second) })
	} else {
		s.exec.Do(func() { s.st.Set(key, string(body)) })
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) getKV(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	var v string
	var ok bool
	var err error
	s.exec.Do(func() { v, ok, err = s.st.Get(key) })
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
	var ok bool
	s.exec.Do(func() { ok = s.st.Del(key) })
	if !ok {
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
	var ok bool
	s.exec.Do(func() { ok = s.st.Expire(key, time.Duration(sec)*time.Second) })
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) ttl(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	var n int64
	s.exec.Do(func() { n = s.st.TTL(key) })
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(w, "%d", n)
}

func (s *Server) putHash(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	field := r.PathValue("field")
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var herr error
	s.exec.Do(func() { _, herr = s.st.HSet(key, field, string(body)) })
	if herr != nil {
		if herr == store.ErrWrongType {
			http.Error(w, herr.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, herr.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) getHashField(w http.ResponseWriter, r *http.Request) {
	var v string
	var ok bool
	var err error
	s.exec.Do(func() { v, ok, err = s.st.HGet(r.PathValue("key"), r.PathValue("field")) })
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
	var all map[string]string
	var err error
	s.exec.Do(func() { all, err = s.st.HGetAll(r.PathValue("key")) })
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
	var zerr error
	s.exec.Do(func() { _, zerr = s.st.ZAdd(r.PathValue("key"), r.PathValue("member"), score) })
	if zerr != nil {
		if zerr == store.ErrWrongType {
			http.Error(w, zerr.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, zerr.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) getZSet(w http.ResponseWriter, r *http.Request) {
	var got []store.ZMember
	var err error
	s.exec.Do(func() { got, err = s.st.ZRange(r.PathValue("key"), 0, -1) })
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

func (s *Server) rewrite(w http.ResponseWriter, _ *http.Request) {
	var err error
	s.exec.Do(func() { err = s.st.Rewrite() })
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
