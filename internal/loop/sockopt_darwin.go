//go:build darwin

package loop

import "golang.org/x/sys/unix"

func extraSockopts(fd int) {
	_ = unix.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_NOSIGPIPE, 1)
}
