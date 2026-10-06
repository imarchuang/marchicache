//go:build linux

package loop

import (
	"time"

	"golang.org/x/sys/unix"
)

type Poll struct {
	fd    int
	evs   []unix.EpollEvent
	wants map[int]uint32
}

func newPoll() (*Poll, error) {
	fd, err := unix.EpollCreate1(unix.EPOLL_CLOEXEC)
	if err != nil {
		return nil, err
	}
	return &Poll{fd: fd, evs: make([]unix.EpollEvent, 128), wants: make(map[int]uint32)}, nil
}

func (p *Poll) ctl(fd int, op int, events uint32) error {
	ev := unix.EpollEvent{Events: events, Fd: int32(fd)}
	return unix.EpollCtl(p.fd, op, fd, &ev)
}

func (p *Poll) addRead(fd int) error {
	p.wants[fd] = unix.EPOLLIN | unix.EPOLLET
	return p.ctl(fd, unix.EPOLL_CTL_ADD, p.wants[fd])
}

func (p *Poll) enableWrite(fd int, on bool) error {
	w := unix.EPOLLIN | unix.EPOLLET
	if on {
		w |= unix.EPOLLOUT
	}
	p.wants[fd] = w
	return p.ctl(fd, unix.EPOLL_CTL_MOD, w)
}

func (p *Poll) del(fd int) error {
	delete(p.wants, fd)
	return unix.EpollCtl(p.fd, unix.EPOLL_CTL_DEL, fd, nil)
}

func (p *Poll) wait(timeout time.Duration) ([]Event, error) {
	ms := int(timeout.Milliseconds())
	if timeout < 0 {
		ms = -1
	}
	n, err := unix.EpollWait(p.fd, p.evs, ms)
	if err != nil {
		if err == unix.EINTR {
			return nil, nil
		}
		return nil, err
	}
	out := make([]Event, 0, n)
	for i := 0; i < n; i++ {
		e := p.evs[i]
		ev := Event{Fd: int(e.Fd)}
		if e.Events&(unix.EPOLLIN|unix.EPOLLHUP|unix.EPOLLERR) != 0 {
			ev.Read = true
		}
		if e.Events&unix.EPOLLOUT != 0 {
			ev.Write = true
		}
		out = append(out, ev)
	}
	return out, nil
}

func (p *Poll) close() error {
	return unix.Close(p.fd)
}
