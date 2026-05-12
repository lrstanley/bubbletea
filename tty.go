package tea

import (
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/term"
)

// ttyInputDrainQuietMillis is how long we wait with no readable input while
// draining (ioctl poll/read deadline). Mirrors the quiet window between
// kernel flushes used on POSIX kernels.
const ttyInputDrainQuietMillis = 200

func (p *Program) suspend() {
	if err := p.releaseTerminal(true); err != nil {
		// If we can't release input, abort.
		return
	}

	suspendProcess()

	_ = p.RestoreTerminal()
	go p.Send(ResumeMsg{})
}

func (p *Program) initTerminal() error {
	if p.disableRenderer {
		return nil
	}
	return p.initInput()
}

// restoreTerminalState restores the terminal to the state prior to running the
// Bubble Tea program.
func (p *Program) restoreTerminalState() error {
	// Flush queued commands.
	_ = p.flush()

	// Drain any pending terminal responses from the TTY input buffer before
	// restoring it. The program may have sent capability queries (e.g.
	// DECRQM for modes 2026/2027) whose responses arrive asynchronously and
	// were not consumed by the cancelled input reader. Without this, those
	// bytes are read by the user's shell after exit and printed as garbage
	// characters. See issue #1590.
	p.drainInput()

	return p.restoreInput()
}

// drainInput clears pending unsolicited terminal replies (DECRPM, etc.).
// It prefers flushing the kernel TTY queue via tryKernelDrainTTY (see drain_*.go);
// when that does not apply or fails (e.g. input is terminal-like but not
// ioctl-flushable), it falls back to a deadline-based Read loop on the active
// input reader.
func (p *Program) drainInput() {
	if p.tryKernelDrainTTY() {
		return
	}
	rd := p.drainReaderSource()
	if rd == nil {
		return
	}
	drainPendingInput(rd, ttyInputDrainQuietMillis*time.Millisecond)
}

func (p *Program) drainReaderSource() io.Reader {
	if p.ttyInput != nil {
		return p.ttyInput
	}
	return p.input
}

type timeoutError interface {
	Timeout() bool
	error
}

// drainPendingInput discards buffered input until silence exceeds [quiet].
// Requires [rd] to support SetReadDeadline; otherwise this is a no-op.
func drainPendingInput(rd io.Reader, quiet time.Duration) {
	dl, ok := rd.(interface {
		SetReadDeadline(time.Time) error
	})
	if !ok {
		return
	}

	buf := make([]byte, 4096)
	for {
		_ = dl.SetReadDeadline(time.Now().Add(quiet))
		n, err := rd.Read(buf)
		if err != nil {
			if errors.Is(err, os.ErrDeadlineExceeded) {
				return
			}
			var te timeoutError
			if errors.As(err, &te) && te.Timeout() {
				return
			}
			return
		}
		if n == 0 {
			return
		}
	}
}

// restoreInput restores the tty input to its original state.
func (p *Program) restoreInput() error {
	if p.ttyInput != nil && p.previousTtyInputState != nil {
		if err := term.Restore(p.ttyInput.Fd(), p.previousTtyInputState); err != nil {
			return fmt.Errorf("bubbletea: error restoring console: %w", err)
		}
	}
	if p.ttyOutput != nil && p.previousOutputState != nil {
		if err := term.Restore(p.ttyOutput.Fd(), p.previousOutputState); err != nil {
			return fmt.Errorf("bubbletea: error restoring console: %w", err)
		}
	}
	return nil
}

// initInputReader (re)commences reading inputs.
func (p *Program) initInputReader(cancel bool) error {
	if cancel && p.cancelReader != nil {
		p.cancelReader.Cancel()
		p.waitForReadLoop()
	}

	term := p.environ.Getenv("TERM")

	// Initialize the input reader.
	// This need to be done after the terminal has been initialized and set to
	// raw mode.

	var err error
	p.cancelReader, err = uv.NewCancelReader(p.input)
	if err != nil {
		return fmt.Errorf("bubbletea: could not create cancelable reader: %w", err)
	}

	drv := uv.NewTerminalReader(p.cancelReader, term)
	drv.SetLogger(p.logger)
	p.inputScanner = drv
	p.readLoopDone = make(chan struct{})

	go p.readLoop()

	return nil
}

func (p *Program) readLoop() {
	defer close(p.readLoopDone)

	if err := p.inputScanner.StreamEvents(p.ctx, p.msgs); err != nil {
		select {
		case <-p.ctx.Done():
			return
		case p.errs <- err:
		}
	}
}

// waitForReadLoop waits for the cancelReader to finish its read loop.
func (p *Program) waitForReadLoop() {
	select {
	case <-p.readLoopDone:
	case <-time.After(500 * time.Millisecond): //nolint:mnd
		// The read loop hangs, which means the input
		// cancelReader's cancel function has returned true even
		// though it was not able to cancel the read.
	}
}

// checkResize detects the current size of the output and informs the program
// via a WindowSizeMsg.
func (p *Program) checkResize() {
	if p.ttyOutput == nil {
		// can't query window size
		return
	}

	w, h, err := term.GetSize(p.ttyOutput.Fd())
	if err != nil {
		select {
		case <-p.ctx.Done():
		case p.errs <- err:
		}

		return
	}

	p.width, p.height = w, h
	p.Send(WindowSizeMsg{Width: w, Height: h})
}

// OpenTTY opens the running terminal's TTY for reading and writing.
func OpenTTY() (*os.File, *os.File, error) {
	in, out, err := uv.OpenTTY()
	if err != nil {
		return nil, nil, fmt.Errorf("bubbletea: could not open TTY: %w", err)
	}
	return in, out, nil
}
