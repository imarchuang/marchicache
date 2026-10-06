package store

import (
	"runtime"
	"strconv"
	"strings"
)

func goid() int64 {
	var buf [64]byte
	n := runtime.Stack(buf[:], false)
	s := string(buf[:n])
	s = strings.TrimPrefix(s, "goroutine ")
	i := strings.IndexByte(s, ' ')
	if i < 0 {
		return 0
	}
	id, _ := strconv.ParseInt(s[:i], 10, 64)
	return id
}

// BindLoop records the event-loop goroutine. Dict methods panic if called from elsewhere.
func (s *Store) BindLoop() {
	s.loopID = goid()
}

func (s *Store) check() {
	if s.loopID == 0 {
		return
	}
	if goid() != s.loopID {
		panic("dict accessed off the event loop")
	}
}
