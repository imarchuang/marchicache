package store

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type FsyncPolicy string

const (
	FsyncAlways   FsyncPolicy = "always"
	FsyncEverysec FsyncPolicy = "everysec"
	FsyncNo       FsyncPolicy = "no"
)

const aofName = "appendonly.aof"

type aofRec struct {
	Op       string  `json:"op"`
	Key      string  `json:"key"`
	Value    string  `json:"value,omitempty"`
	Field    string  `json:"field,omitempty"`
	Member   string  `json:"member,omitempty"`
	Score    float64 `json:"score,omitempty"`
	ExpireAt int64   `json:"expireAt"`
}

func ParseFsync(s string) (FsyncPolicy, error) {
	switch FsyncPolicy(s) {
	case FsyncAlways, FsyncEverysec, FsyncNo:
		return FsyncPolicy(s), nil
	case "":
		return FsyncEverysec, nil
	default:
		return "", fmt.Errorf("unknown appendfsync %q", s)
	}
}

func Open(dataDir string, policy FsyncPolicy) (*Store, error) {
	return OpenWithClock(dataDir, policy, realClock{})
}

func OpenWithClock(dataDir string, policy FsyncPolicy, clock Clock) (*Store, error) {
	if clock == nil {
		clock = realClock{}
	}
	if policy == "" {
		policy = FsyncEverysec
	}
	s := &Store{
		dict:    make(map[string]*entry),
		clock:   clock,
		dataDir: dataDir,
		policy:  policy,
	}
	if dataDir == "" {
		return s, nil
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(dataDir, aofName)
	if err := s.replay(path); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	s.aof = f
	if policy == FsyncEverysec {
		s.fsyncStop = make(chan struct{})
		s.fsyncWG.Add(1)
		go s.everysecLoop(s.fsyncStop)
	}
	return s, nil
}

func (s *Store) replay(path string) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var rec aofRec
		if err := json.Unmarshal(line, &rec); err != nil {
			return fmt.Errorf("aof replay: %w", err)
		}
		s.applyRec(rec)
	}
	return sc.Err()
}

func (s *Store) applyRec(rec aofRec) {
	switch rec.Op {
	case "SET":
		e := &entry{typ: typeString, value: rec.Value}
		if rec.ExpireAt > 0 {
			e.expireAt = time.Unix(0, rec.ExpireAt)
			if s.isExpiredLocked(e) {
				return
			}
		}
		s.dict[rec.Key] = e
	case "DEL":
		delete(s.dict, rec.Key)
	case "EXPIRE":
		e, ok := s.dict[rec.Key]
		if !ok {
			return
		}
		if rec.ExpireAt <= 0 {
			delete(s.dict, rec.Key)
			return
		}
		e.expireAt = time.Unix(0, rec.ExpireAt)
		if s.isExpiredLocked(e) {
			delete(s.dict, rec.Key)
		}
	case "HSET":
		e, ok := s.dict[rec.Key]
		if !ok {
			e = &entry{typ: typeHash, hash: make(map[string]string)}
			s.dict[rec.Key] = e
		}
		if e.hash == nil {
			e.hash = make(map[string]string)
		}
		e.typ = typeHash
		e.hash[rec.Field] = rec.Value
	case "ZADD":
		e, ok := s.dict[rec.Key]
		if !ok {
			e = &entry{typ: typeZSet}
			s.dict[rec.Key] = e
		}
		e.typ = typeZSet
		insertZ(e, rec.Member, rec.Score)
	}
}

func (s *Store) appendLocked(rec aofRec) {
	if s.aof == nil {
		return
	}
	b, err := json.Marshal(rec)
	if err != nil {
		s.aofErr = err
		return
	}
	b = append(b, '\n')
	if _, err := s.aof.Write(b); err != nil {
		s.aofErr = err
		return
	}
	s.aofDirty = true
	if s.policy == FsyncAlways {
		if err := s.aof.Sync(); err != nil {
			s.aofErr = err
			return
		}
		s.aofDirty = false
	}
}

func (s *Store) everysecLoop(stop <-chan struct{}) {
	defer s.fsyncWG.Done()
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			s.mu.Lock()
			s.fsyncIfDirtyLocked()
			s.mu.Unlock()
		}
	}
}

func (s *Store) fsyncIfDirtyLocked() {
	if s.aof == nil || !s.aofDirty {
		return
	}
	if err := s.aof.Sync(); err != nil {
		s.aofErr = err
		return
	}
	s.aofDirty = false
}

func (s *Store) stopFsyncLocked() {
	if s.fsyncStop == nil {
		return
	}
	close(s.fsyncStop)
	s.fsyncStop = nil
	s.mu.Unlock()
	s.fsyncWG.Wait()
	s.mu.Lock()
}

// Close fsyncs then closes the AOF. Tests simulating kill should not call this.
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopFsyncLocked()
	if s.aof == nil {
		return nil
	}
	s.fsyncIfDirtyLocked()
	err := s.aof.Close()
	s.aof = nil
	return err
}

// KillClose closes the AOF fd without an extra fsync (crash without clean shutdown).
func (s *Store) KillClose() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopFsyncLocked()
	if s.aof == nil {
		return nil
	}
	err := s.aof.Close()
	s.aof = nil
	return err
}

func (s *Store) AOFBytes() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.aof == nil {
		return 0
	}
	st, err := s.aof.Stat()
	if err != nil {
		return 0
	}
	return st.Size()
}

func (s *Store) FsyncPolicy() string {
	if s.policy == "" {
		return "none"
	}
	return string(s.policy)
}

func expireUnix(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixNano()
}
