//go:build darwin || linux

package loop

import (
	"net"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/marchi/marchicache/internal/store"
	"golang.org/x/sys/unix"
)

type job struct {
	fn   func()
	done chan struct{}
}

type client struct {
	fd    int
	in    []byte
	out   []byte
	wantW bool
}

// Reactor is one goroutine: poll + execute. The dict has no mutex.
type Reactor struct {
	st      *store.Store
	poll    *Poll
	ln      net.Listener
	lnFile  *os.File
	lfd     int
	wakeR   int
	wakeW   int
	clients map[int]*client
	jobs    chan job
	stop    chan struct{}
	loopID  int64
	Addr    string
	ready   chan struct{}
	stopped atomic.Bool
}

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

func New(st *store.Store, redisAddr string) (*Reactor, error) {
	p, err := newPoll()
	if err != nil {
		return nil, err
	}
	var fds [2]int
	if err := unix.Pipe(fds[:]); err != nil {
		p.close()
		return nil, err
	}
	if err := unix.SetNonblock(fds[0], true); err != nil {
		return nil, err
	}
	if err := unix.SetNonblock(fds[1], true); err != nil {
		return nil, err
	}
	r := &Reactor{
		st:      st,
		poll:    p,
		lfd:     -1,
		wakeR:   fds[0],
		wakeW:   fds[1],
		clients: make(map[int]*client),
		jobs:    make(chan job, 256),
		stop:    make(chan struct{}),
		ready:   make(chan struct{}),
	}
	if err := p.addRead(r.wakeR); err != nil {
		return nil, err
	}
	if redisAddr != "" {
		ln, err := net.Listen("tcp", redisAddr)
		if err != nil {
			return nil, err
		}
		r.ln = ln
		r.Addr = ln.Addr().String()
		f, err := ln.(*net.TCPListener).File()
		if err != nil {
			ln.Close()
			return nil, err
		}
		_ = ln.Close()
		r.ln = nil
		r.lnFile = f
		r.lfd = int(f.Fd())
		if err := unix.SetNonblock(r.lfd, true); err != nil {
			return nil, err
		}
		if err := p.addRead(r.lfd); err != nil {
			return nil, err
		}
	}
	return r, nil
}

func (r *Reactor) Start() {
	go r.Run()
	<-r.ready
}

func (r *Reactor) Run() {
	r.st.BindLoop()
	r.loopID = goid()
	close(r.ready)
	for !r.stopped.Load() {
		evs, err := r.poll.wait(50 * time.Millisecond)
		if err != nil {
			continue
		}
		r.drainJobs()
		r.st.Tick()
		for _, ev := range evs {
			switch {
			case ev.Fd == r.wakeR:
				r.drainWake()
			case r.lfd >= 0 && ev.Fd == r.lfd:
				r.acceptAll()
			default:
				c := r.clients[ev.Fd]
				if c == nil {
					continue
				}
				if ev.Read {
					r.onRead(c)
				}
				if ev.Write && r.clients[c.fd] != nil {
					r.onWrite(c)
				}
			}
		}
	}
}

func (r *Reactor) Do(fn func()) {
	if r.loopID != 0 && goid() == r.loopID {
		fn()
		return
	}
	if r.stopped.Load() {
		return
	}
	j := job{fn: fn, done: make(chan struct{})}
	select {
	case r.jobs <- j:
		r.kick()
	case <-r.stop:
		return
	}
	<-j.done
}

func (r *Reactor) Stop() {
	r.Do(func() {
		r.stopped.Store(true)
	})
}

func (r *Reactor) kick() {
	_, _ = unix.Write(r.wakeW, []byte{1})
}

func (r *Reactor) drainWake() {
	var b [64]byte
	for {
		_, err := unix.Read(r.wakeR, b[:])
		if err != nil {
			break
		}
	}
}

func (r *Reactor) drainJobs() {
	for {
		select {
		case j := <-r.jobs:
			j.fn()
			close(j.done)
		default:
			return
		}
	}
}

func (r *Reactor) acceptAll() {
	for {
		nfd, _, err := unix.Accept(r.lfd)
		if err != nil {
			if err == unix.EAGAIN || err == unix.EWOULDBLOCK {
				return
			}
			return
		}
		_ = unix.SetNonblock(nfd, true)
		extraSockopts(nfd)
		if err := r.poll.addRead(nfd); err != nil {
			_ = unix.Close(nfd)
			continue
		}
		r.clients[nfd] = &client{fd: nfd}
	}
}

func (r *Reactor) onRead(c *client) {
	buf := make([]byte, 8192)
	for {
		n, err := unix.Read(c.fd, buf)
		if n > 0 {
			c.in = append(c.in, buf[:n]...)
		}
		if err != nil {
			if err == unix.EAGAIN || err == unix.EWOULDBLOCK {
				break
			}
			r.closeClient(c)
			return
		}
		if n == 0 {
			r.closeClient(c)
			return
		}
		if n < len(buf) {
			break
		}
	}
	for {
		args, rest, ok, err := parseRESP(c.in)
		if err != nil {
			c.out = append(c.out, errStr("protocol error")...)
			r.closeClient(c)
			return
		}
		if !ok {
			break
		}
		c.in = rest
		if len(args) == 0 {
			continue
		}
		c.out = append(c.out, dispatch(r.st, args)...)
	}
	r.onWrite(c)
}

func (r *Reactor) onWrite(c *client) {
	for len(c.out) > 0 {
		n, err := unix.Write(c.fd, c.out)
		if n > 0 {
			c.out = c.out[n:]
		}
		if err != nil {
			if err == unix.EAGAIN || err == unix.EWOULDBLOCK {
				if !c.wantW {
					_ = r.poll.enableWrite(c.fd, true)
					c.wantW = true
				}
				return
			}
			r.closeClient(c)
			return
		}
		if n == 0 {
			return
		}
	}
	if c.wantW {
		_ = r.poll.enableWrite(c.fd, false)
		c.wantW = false
	}
}

func (r *Reactor) closeClient(c *client) {
	_ = r.poll.del(c.fd)
	_ = unix.Close(c.fd)
	delete(r.clients, c.fd)
}
