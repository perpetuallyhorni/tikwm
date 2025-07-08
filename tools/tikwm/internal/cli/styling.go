package cli

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/fatih/color"
)

// Console manages styled and dynamic CLI output.
type Console struct {
	mu         sync.Mutex   // mu protects concurrent access to the console's state.
	spinner    *spinner     // spinner holds the spinner animation data.
	spinnerMsg string       // spinnerMsg stores the message displayed with the spinner.
	isSpinning bool         // isSpinning indicates whether the spinner is currently active.
	isQuiet    bool         // isQuiet suppresses all output if set to true.
	Bold       *color.Color // Bold is a color object for bold text.
	Green      *color.Color // Green is a color object for green text.
	Yellow     *color.Color // Yellow is a color object for yellow text.
	Red        *color.Color // Red is a color object for red text.
	Cyan       *color.Color // Cyan is a color object for cyan text.
}

// New creates a new Console.
func New(quiet bool) *Console {
	return &Console{
		isQuiet: quiet,
		Bold:    color.New(color.Bold),
		Green:   color.New(color.FgGreen),
		Yellow:  color.New(color.FgYellow),
		Red:     color.New(color.FgRed),
		Cyan:    color.New(color.FgCyan),
	}
}

// Info prints a standard informational message.
func (c *Console) Info(format string, a ...interface{}) {
	if c.isQuiet {
		return
	}
	c.stopSpinner()
	fmt.Fprintf(os.Stderr, "%s\n", fmt.Sprintf(format, a...))
}

// Success prints a success message.
func (c *Console) Success(format string, a ...interface{}) {
	if c.isQuiet {
		return
	}
	c.stopSpinner()
	_, _ = c.Green.Fprintf(os.Stderr, "✓ %s\n", fmt.Sprintf(format, a...))
}

// Warn prints a warning message.
func (c *Console) Warn(format string, a ...interface{}) {
	if c.isQuiet {
		return
	}
	c.stopSpinner()
	_, _ = c.Yellow.Fprintf(os.Stderr, "! %s\n", fmt.Sprintf(format, a...))
}

// Error prints an error message.
func (c *Console) Error(format string, a ...interface{}) {
	if c.isQuiet {
		return
	}
	c.stopSpinner()
	_, _ = c.Red.Fprintf(os.Stderr, "✗ %s\n", fmt.Sprintf(format, a...))
}

// StartProgress starts a dynamic progress line with a spinner.
func (c *Console) StartProgress(message string) {
	if c.isQuiet {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.isSpinning {
		c.stopSpinnerInternal()
	}

	c.spinner = newSpinner()
	c.spinnerMsg = message
	c.isSpinning = true

	go func() {
		for {
			c.mu.Lock()
			if !c.isSpinning {
				c.mu.Unlock()
				return
			}
			frame := c.spinner.next()
			fmt.Fprintf(os.Stderr, "\r\033[K%s %s", c.Green.Sprint(frame), c.spinnerMsg)
			c.mu.Unlock()
			time.Sleep(100 * time.Millisecond)
		}
	}()
}

// UpdateProgress updates the message of the current progress line.
func (c *Console) UpdateProgress(message string) {
	if c.isQuiet || !c.isSpinning {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.spinnerMsg = message
}

// StopProgress stops the progress line.
func (c *Console) StopProgress() {
	if c.isQuiet {
		return
	}
	c.stopSpinner()
}

// stopSpinner is a thread-safe wrapper around stopSpinnerInternal.
func (c *Console) stopSpinner() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stopSpinnerInternal()
}

// stopSpinnerInternal stops the spinner and clears the current line.
func (c *Console) stopSpinnerInternal() {
	if c.isSpinning {
		c.isSpinning = false
		fmt.Fprint(os.Stderr, "\r\033[K")
	}
}

// spinner manages the animation frames for a spinner.
type spinner struct {
	frames []string // frames is a slice of strings representing the spinner animation.
	index  int      // index is the current frame index.
}

// newSpinner creates a new spinner with a default set of frames.
func newSpinner() *spinner {
	return &spinner{
		frames: []string{"⣷", "⣯", "⣟", "⡿", "⢿", "⣻", "⣽", "⣾"},
	}
}

// next returns the next frame in the spinner animation.
func (s *spinner) next() string {
	frame := s.frames[s.index]
	s.index = (s.index + 1) % len(s.frames)
	return frame
}
