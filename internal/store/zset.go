package store

import "sort"

func insertZ(e *entry, member string, score float64) int {
	added := 1
	for i, m := range e.zset {
		if m.Member == member {
			e.zset = append(e.zset[:i], e.zset[i+1:]...)
			added = 0
			break
		}
	}
	e.zset = append(e.zset, ZMember{Member: member, Score: score})
	sort.SliceStable(e.zset, func(i, j int) bool {
		if e.zset[i].Score == e.zset[j].Score {
			return e.zset[i].Member < e.zset[j].Member
		}
		return e.zset[i].Score < e.zset[j].Score
	})
	return added
}

func (s *Store) ZAdd(key, member string, score float64) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.lookupLocked(key)
	if !ok {
		e = &entry{typ: typeZSet, lastAccess: s.clock.Now()}
		s.dict[key] = e
	} else if e.typ != typeZSet {
		return 0, ErrWrongType
	}
	added := insertZ(e, member, score)
	s.appendLocked(aofRec{Op: "ZADD", Key: key, Member: member, Score: score})
	s.evictLRULocked()
	return added, nil
}

func (s *Store) ZRange(key string, start, stop int) ([]ZMember, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.lookupLocked(key)
	if !ok {
		return nil, nil
	}
	if e.typ != typeZSet {
		return nil, ErrWrongType
	}
	n := len(e.zset)
	if n == 0 {
		return nil, nil
	}
	if start < 0 {
		start = n + start
	}
	if stop < 0 {
		stop = n + stop
	}
	if start < 0 {
		start = 0
	}
	if stop >= n {
		stop = n - 1
	}
	if start > stop || start >= n {
		return nil, nil
	}
	out := make([]ZMember, stop-start+1)
	copy(out, e.zset[start:stop+1])
	return out, nil
}
