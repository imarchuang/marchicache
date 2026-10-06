//go:build darwin

package loop

import (
	"time"

	"golang.org/x/sys/unix"
)

type Poll struct {
	kq int
}

func newPoll() (*Poll, error) {
	kq, err := unix.Kqueue()
	if err != nil {
		return nil, err
	}
	return &Poll{kq: kq}, nil
}

func (p *Poll) addRead(fd int) error {
	ev := unix.Kevent_t{
		Ident:  uint64(fd),
		Filter: unix.EVFILT_READ,
		Flags:  unix.EV_ADD | unix.EV_CLEAR,
	}
	_, err := unix.Kevent(p.kq, []unix.Kevent_t{ev}, nil, nil)
	return err
}

func (p *Poll) enableWrite(fd int, on bool) error {
	ev := unix.Kevent_t{
		Ident:  uint64(fd),
		Filter: unix.EVFILT_WRITE,
	}
	if on {
		ev.Flags = unix.EV_ADD | unix.EV_CLEAR
	} else {
		ev.Flags = unix.EV_DELETE
	}
	_, err := unix.Kevent(p.kq, []unix.Kevent_t{ev}, nil, nil)
	if err == unix.ENOENT && !on {
		return nil
	}
	return err
}

func (p *Poll) del(fd int) error {
	evs := []unix.Kevent_t{
		{Ident: uint64(fd), Filter: unix.EVFILT_READ, Flags: unix.EV_DELETE},
		{Ident: uint64(fd), Filter: unix.EVFILT_WRITE, Flags: unix.EV_DELETE},
	}
	_, err := unix.Kevent(p.kq, evs, nil, nil)
	return err
}

func (p *Poll) wait(timeout time.Duration) ([]Event, error) {
	var ts unix.Timespec
	if timeout >= 0 {
		ts = unix.NsecToTimespec(timeout.Nanoseconds())
	}
	buf := make([]unix.Kevent_t, 128)
	var tsp *unix.Timespec
	if timeout >= 0 {
		tsp = &ts
	}
	n, err := unix.Kevent(p.kq, nil, buf, tsp)
	if err != nil {
		if err == unix.EINTR {
			return nil, nil
		}
		return nil, err
	}
	out := make([]Event, 0, n)
	for i := 0; i < n; i++ {
		e := buf[i]
		ev := Event{Fd: int(e.Ident)}
		switch e.Filter {
		case unix.EVFILT_READ:
			ev.Read = true
		case unix.EVFILT_WRITE:
			ev.Write = true
		}
		out = append(out, ev)
	}
	return out, nil
}

func (p *Poll) close() error {
	return unix.Close(p.kq)
}
