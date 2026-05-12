//go:build !windows && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris && !aix
// +build !windows,!darwin,!dragonfly,!freebsd,!linux,!netbsd,!openbsd,!solaris,!aix

package tea

// tryKernelDrainTTY is unavailable on these platforms — read-based drain handles
// pending input where the reader supports SetReadDeadline.
func (*Program) tryKernelDrainTTY() bool { return false }
