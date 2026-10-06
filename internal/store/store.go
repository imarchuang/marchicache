package store

import "sync"

// Store is an in-memory string dict. Slice 0 has no TTL or AOF.
type Store struct {
	mu   sync.RWMutex
	dict map[string]string
}

func New() *Store {
	return &Store{dict: make(map[string]string)}
}

func (s *Store) Set(key, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dict[key] = value
}

func (s *Store) Get(key string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.dict[key]
	return v, ok
}

func (s *Store) Del(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.dict[key]; !ok {
		return false
	}
	delete(s.dict, key)
	return true
}

func (s *Store) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.dict)
}
