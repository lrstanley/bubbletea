//go:build windows
// +build windows

package tea

import "golang.org/x/sys/windows"

// tryKernelDrainTTY discards queued console input. Returns false when
// [ttyInput] is nil or flushing fails so [drainPendingInput] can run.
func (p *Program) tryKernelDrainTTY() bool {
	if p.ttyInput == nil {
		return false
	}
	err := windows.FlushConsoleInputBuffer(windows.Handle(p.ttyInput.Fd()))
	return err == nil
}
