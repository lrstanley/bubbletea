//go:build linux || solaris || aix
// +build linux solaris aix

package tea

import "golang.org/x/sys/unix"

// tryKernelDrainTTY flushes queued input on an ioctl-capable tty fd. Returns
// true when the ioctl path succeeded (possibly after draining multiple bursts).
// Returns false when [ttyInput] is nil or the fd is notioctl-flushable,
// prompting a deadline-based Read drain.
func (p *Program) tryKernelDrainTTY() bool {
	if p.ttyInput == nil {
		return false
	}
	fd := int(p.ttyInput.Fd())
	fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}} //nolint:gosec // tty fd never overflows int32

	for {
		if err := unix.IoctlSetInt(fd, unix.TCFLSH, 0); err != nil { // TCIFLUSH: discard input
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
