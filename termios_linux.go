//go:build linux

package main

import "golang.org/x/sys/unix"

func setRawModeReadTimeout(fd int) error {
	termios, err := unix.IoctlGetTermios(fd, unix.TCGETS)
	if err != nil {
		return err
	}

	termios.Cc[unix.VMIN] = 0
	termios.Cc[unix.VTIME] = 1

	return unix.IoctlSetTermios(fd, unix.TCSETS, termios)
}
