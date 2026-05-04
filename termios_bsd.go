//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package main

import "golang.org/x/sys/unix"

func setRawModeReadTimeout(fd int) error {
	termios, err := unix.IoctlGetTermios(fd, unix.TIOCGETA)
	if err != nil {
		return err
	}

	termios.Cc[unix.VMIN] = 0
	termios.Cc[unix.VTIME] = 1

	return unix.IoctlSetTermios(fd, unix.TIOCSETA, termios)
}
