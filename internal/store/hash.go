package store

func (s *Store) HSet(key, field, value string) (int, error) {
	s.check()
	e, ok := s.lookupLocked(key)
	if !ok {
		e = &entry{typ: typeHash, hash: make(map[string]string), lastAccess: s.clock.Now()}
		s.dict[key] = e
	} else if e.typ != typeHash {
		return 0, ErrWrongType
	}
	_, exists := e.hash[field]
	e.hash[field] = value
	s.appendLocked(aofRec{Op: "HSET", Key: key, Field: field, Value: value})
	s.evictLRULocked()
	if exists {
		return 0, nil
	}
	return 1, nil
}

func (s *Store) HGet(key, field string) (string, bool, error) {
	s.check()
	e, ok := s.lookupLocked(key)
	if !ok {
		return "", false, nil
	}
	if e.typ != typeHash {
		return "", false, ErrWrongType
	}
	v, ok := e.hash[field]
	return v, ok, nil
}

func (s *Store) HGetAll(key string) (map[string]string, error) {
	s.check()
	e, ok := s.lookupLocked(key)
	if !ok {
		return map[string]string{}, nil
	}
	if e.typ != typeHash {
		return nil, ErrWrongType
	}
	out := make(map[string]string, len(e.hash))
	for f, v := range e.hash {
		out[f] = v
	}
	return out, nil
}
