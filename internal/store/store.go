package store

import (
	"errors"
	"os"
	"sync"
	"time"
)

const (
	TTLMissing  int64 = -2
	TTLNoExpire int64 = -1
)

var ErrWrongType = errors.New("WRONGTYPE Operation against a key holding the wrong kind of value")

// Clock lets tests inject a fake now.
type Clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

type valueType int

const (
	typeString valueType = iota
	typeHash
	typeZSet
)

type ZMember struct {
	Member string
	Score  float64
}

type entry struct {
	typ        valueType
	value      string
	hash       map[string]string
	zset       []ZMember
	expireAt   time.Time
	lastAccess time.Time
}

// Store is an in-memory string dict with per-key TTL and optional AOF.
type Store struct {
	mu        sync.RWMutex
	dict      map[string]*entry
	clock     Clock
	dataDir   string
	policy    FsyncPolicy
	aof       *os.File
	aofDirty  bool
	aofErr    error
	fsyncStop chan struct{}
	fsyncWG   sync.WaitGroup
	maxMemory int64
}

func New() *Store {
	return NewWithClock(realClock{})
}

func NewWithClock(c Clock) *Store {
	return &Store{dict: make(map[string]*entry), clock: c}
}

func (s *Store) Set(key, value string) {
	s.SetEX(key, value, 0)
}

func (s *Store) SetEX(key, value string, ttl time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := &entry{typ: typeString, value: value, lastAccess: s.clock.Now()}
	if ttl > 0 {
		e.expireAt = s.clock.Now().Add(ttl)
	}
	s.dict[key] = e
	s.appendLocked(aofRec{Op: "SET", Key: key, Value: value, ExpireAt: expireUnix(e.expireAt)})
	s.evictLRULocked()
}

func (s *Store) Get(key string) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.lookupLocked(key)
	if !ok {
		return "", false, nil
	}
	if e.typ != typeString {
		return "", false, ErrWrongType
	}
	return e.value, true, nil
}

func (s *Store) lookupLocked(key string) (*entry, bool) {
	e, ok := s.dict[key]
	if !ok {
		return nil, false
	}
	if s.isExpiredLocked(e) {
		delete(s.dict, key)
		s.appendLocked(aofRec{Op: "DEL", Key: key})
		return nil, false
	}
	e.lastAccess = s.clock.Now()
	return e, true
}

func (s *Store) Del(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.dict[key]
	if !ok {
		return false
	}
	if s.isExpiredLocked(e) {
		delete(s.dict, key)
		s.appendLocked(aofRec{Op: "DEL", Key: key})
		return false
	}
	delete(s.dict, key)
	s.appendLocked(aofRec{Op: "DEL", Key: key})
	return true
}

// Expire sets a TTL. Returns false if the key is missing (Redis EXPIRE).
func (s *Store) Expire(key string, ttl time.Duration) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.dict[key]
	if !ok {
		return false
	}
	if s.isExpiredLocked(e) {
		delete(s.dict, key)
		s.appendLocked(aofRec{Op: "DEL", Key: key})
		return false
	}
	if ttl <= 0 {
		delete(s.dict, key)
		s.appendLocked(aofRec{Op: "DEL", Key: key})
		return true
	}
	e.expireAt = s.clock.Now().Add(ttl)
	s.appendLocked(aofRec{Op: "EXPIRE", Key: key, ExpireAt: expireUnix(e.expireAt)})
	return true
}

// TTL returns remaining seconds, -1 if no expire, -2 if missing (Redis TTL).
func (s *Store) TTL(key string) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.dict[key]
	if !ok {
		return TTLMissing
	}
	if s.isExpiredLocked(e) {
		delete(s.dict, key)
		s.appendLocked(aofRec{Op: "DEL", Key: key})
		return TTLMissing
	}
	if e.expireAt.IsZero() {
		return TTLNoExpire
	}
	sec := int64(e.expireAt.Sub(s.clock.Now()).Seconds())
	if sec < 0 {
		delete(s.dict, key)
		s.appendLocked(aofRec{Op: "DEL", Key: key})
		return TTLMissing
	}
	return sec
}

func (s *Store) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lazySweepLocked()
	return len(s.dict)
}

func (s *Store) isExpiredLocked(e *entry) bool {
	if e.expireAt.IsZero() {
		return false
	}
	return !s.clock.Now().Before(e.expireAt)
}

func (s *Store) lazySweepLocked() {
	for k, e := range s.dict {
		if s.isExpiredLocked(e) {
			delete(s.dict, k)
			s.appendLocked(aofRec{Op: "DEL", Key: k})
		}
	}
}

// ActiveExpire samples up to limit keys (Go map iteration is randomized)
// and deletes expired ones. Redis-style active expire on a time event.
func (s *Store) ActiveExpire(limit int) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 {
		limit = 20
	}
	n := 0
	i := 0
	for k, e := range s.dict {
		if i >= limit {
			break
		}
		i++
		if s.isExpiredLocked(e) {
			delete(s.dict, k)
			s.appendLocked(aofRec{Op: "DEL", Key: k})
			n++
		}
	}
	return n
}

func (s *Store) StartActiveExpire(stop <-chan struct{}, interval time.Duration) {
	if interval <= 0 {
		interval = 100 * time.Millisecond
	}
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				s.ActiveExpire(20)
			}
		}
	}()
}
