//go:build darwin || dragonfly || freebsd || netbsd || openbsd
// +build darwin dragonfly freebsd netbsd openbsd

package tea

import "golang.org/x/sys/unix"

// tryKernelDrainTTY flushes queued input on an ioctl-capable tty fd. Returns
// true when the ioctl path succeeded (possibly after draining multiple bursts).
// Returns false when [ttyInput] is nil or the fd is not ioctl-flushable,
// prompting a deadline-based Read drain.
func (p *Program) tryKernelDrainTTY() bool {
	if p.ttyInput == nil {
		return false
	}
	fd := int(p.ttyInput.Fd())
	fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}} //nolint:gosec // tty fd never overflows int32

	for {
		// FREAD (1) tells TIOCFLUSH to discard the read queue only.
		if err := unix.IoctlSetPointerInt(fd, unix.TIOCFLUSH, 1); err != nil {
			return false
		}

		n, err := unix.Poll(fds, drainTimeoutMs)
		if err != nil {
			return false
		}
		if n <= 0 {
			return true
		}
	}
}
