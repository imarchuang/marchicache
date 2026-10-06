package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const rewriteName = "rewrite.tmp"

func (s *Store) dumpRecordsLocked() []aofRec {
	var recs []aofRec
	for k, e := range s.dict {
		if s.isExpiredLocked(e) {
			continue
		}
		switch e.typ {
		case typeHash:
			for f, v := range e.hash {
				recs = append(recs, aofRec{Op: "HSET", Key: k, Field: f, Value: v, ExpireAt: expireUnix(e.expireAt)})
			}
		case typeZSet:
			for _, z := range e.zset {
				recs = append(recs, aofRec{Op: "ZADD", Key: k, Member: z.Member, Score: z.Score, ExpireAt: expireUnix(e.expireAt)})
			}
		default:
			recs = append(recs, aofRec{Op: "SET", Key: k, Value: e.value, ExpireAt: expireUnix(e.expireAt)})
		}
	}
	return recs
}

// Rewrite dumps the live dict to rewrite.tmp and atomically replaces appendonly.aof.
func (s *Store) Rewrite() error {
	s.check()
	if s.dataDir == "" {
		return fmt.Errorf("rewrite requires dataDir")
	}
	tmp := filepath.Join(s.dataDir, rewriteName)
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	for _, rec := range s.dumpRecordsLocked() {
		if err := enc.Encode(rec); err != nil {
			f.Close()
			return err
		}
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	final := filepath.Join(s.dataDir, aofName)
	if s.aof != nil {
		_ = s.aof.Close()
		s.aof = nil
	}
	if err := os.Rename(tmp, final); err != nil {
		return err
	}
	af, err := os.OpenFile(final, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	s.aof = af
	s.aofDirty = false
	return nil
}
