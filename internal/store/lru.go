package store

import "time"

func (s *Store) SetMaxMemory(n int64) {
	s.check()
	s.maxMemory = n
	s.evictLRULocked()
}

func (e *entry) nbytes() int {
	n := 64
	n += len(e.value)
	for k, v := range e.hash {
		n += len(k) + len(v)
	}
	for _, z := range e.zset {
		n += len(z.Member) + 8
	}
	return n
}

func (s *Store) usedMemoryLocked() int64 {
	var n int64
	for k, e := range s.dict {
		n += int64(len(k) + e.nbytes())
	}
	return n
}

func (s *Store) UsedMemory() int64 {
	s.check()
	return s.usedMemoryLocked()
}

// evictLRULocked samples keys and drops the coldest until under maxmemory (allkeys-lru approx).
func (s *Store) evictLRULocked() {
	if s.maxMemory <= 0 {
		return
	}
	for s.usedMemoryLocked() > s.maxMemory && len(s.dict) > 0 {
		victim := ""
		var oldest time.Time
		i := 0
		for k, e := range s.dict {
			if i >= 5 {
				break
			}
			i++
			if victim == "" || e.lastAccess.Before(oldest) {
				victim = k
				oldest = e.lastAccess
			}
		}
		if victim == "" {
			return
		}
		delete(s.dict, victim)
		s.appendLocked(aofRec{Op: "DEL", Key: victim})
	}
}
