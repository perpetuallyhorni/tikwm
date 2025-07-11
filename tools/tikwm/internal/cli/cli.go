package cli

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/fatih/color"
)

// OperationType categorizes the work a task is doing.
type OperationType int

const (
	// OpUnknown is the default, un-categorized operation type.
	OpUnknown OperationType = iota
	// OpFeedFetch represents a task fetching post metadata.
	OpFeedFetch
	// OpDownload represents a task downloading a media file.
	OpDownload
)

// opState holds the operational state of a task for UI rendering.
type opState struct {
	opType       OperationType
	lastActivity time.Time
}

// spinner manages the animation frames for a spinner.
type spinner struct {
	frames []string
	index  int
}

func newSpinner() *spinner {
	return &spinner{
		frames: []string{"⣷", "⣯", "⣟", "⡿", "⢿", "⣻", "⣽", "⣾"},
	}
}
func (s *spinner) next() string {
	frame := s.frames[s.index]
	s.index = (s.index + 1) % len(s.frames)
	return frame
}
func (s *spinner) current() string {
	return s.frames[s.index]
}

// managedTask holds the state for a single line in the console.
type managedTask struct {
	id      string
	msg     string
	state   opState
	spinner *spinner
}

// Console manages styled and dynamic CLI output.
type Console struct {
	mu          sync.Mutex
	tasks       map[string]*managedTask
	taskOrder   []string // Ensures stable render order
	isRendering bool
	isQuiet     bool
	lastHeight  int
	// Colors
	Bold      *color.Color
	White     *color.Color
	Lime      *color.Color
	DarkGreen *color.Color
	Yellow    *color.Color
	Cyan      *color.Color
	Gray      *color.Color
	Orange    *color.Color
}

// New creates a new Console.
func New(quiet bool) *Console {
	return &Console{
		isQuiet:   quiet,
		tasks:     make(map[string]*managedTask),
		taskOrder: make([]string, 0),
		// Standard
		Bold:  color.New(color.Bold),
		White: color.New(color.FgWhite),
		// Custom
		Lime:      color.New(color.FgHiGreen),
		DarkGreen: color.New(color.FgGreen),
		Yellow:    color.New(color.FgHiYellow),
		Cyan:      color.New(color.FgCyan),
		Gray:      color.New(color.FgHiBlack),
		Orange:    color.New(color.FgYellow),
	}
}

func (c *Console) printStatic(msg string) {
	if c.isQuiet {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	// Clear any existing dynamic lines before printing a static message.
	if c.lastHeight > 0 {
		fmt.Fprintf(os.Stderr, "\033[%dA\033[J", c.lastHeight)
	}
	c.lastHeight = 0
	fmt.Fprintln(os.Stderr, msg)
}

// Info, Success, Warn, Error methods for static messages
func (c *Console) Info(format string, a ...interface{}) { c.printStatic(fmt.Sprintf(format, a...)) }
func (c *Console) Success(format string, a ...interface{}) {
	c.printStatic(c.Lime.Sprintf("✓ %s", fmt.Sprintf(format, a...)))
}
func (c *Console) Warn(format string, a ...interface{}) {
	c.printStatic(c.Yellow.Sprintf("! %s", fmt.Sprintf(format, a...)))
}
func (c *Console) Error(format string, a ...interface{}) {
	c.printStatic(c.Orange.Sprintf("✗ %s", fmt.Sprintf(format, a...)))
}

// AddTask adds a new task to the multi-line display.
func (c *Console) AddTask(taskID, message string, opType OperationType) {
	if c.isQuiet {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.tasks[taskID]; !exists {
		c.tasks[taskID] = &managedTask{
			id:      taskID,
			msg:     message,
			state:   opState{opType: opType, lastActivity: time.Time{}}, // Zero time indicates initial idle state
			spinner: newSpinner(),
		}
		c.taskOrder = append(c.taskOrder, taskID)
	}

	if !c.isRendering {
		c.isRendering = true
		go c.render()
	}
}

// UpdateTaskMessage updates the message for an existing task.
func (c *Console) UpdateTaskMessage(taskID, message string) {
	if c.isQuiet {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if task, ok := c.tasks[taskID]; ok {
		task.msg = message
	}
}

// UpdateTaskActivity signals that a task is active, resetting its idle timer.
func (c *Console) UpdateTaskActivity(taskID string) {
	if c.isQuiet {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if task, ok := c.tasks[taskID]; ok {
		task.state.lastActivity = time.Now()
	}
}

// RemoveTask removes a task from the display.
func (c *Console) RemoveTask(taskID string) {
	if c.isQuiet {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.tasks, taskID)
	for i, id := range c.taskOrder {
		if id == taskID {
			c.taskOrder = append(c.taskOrder[:i], c.taskOrder[i+1:]...)
			break
		}
	}
}

// StopRenderer signals the rendering goroutine to stop.
func (c *Console) StopRenderer() {
	if c.isQuiet || !c.isRendering {
		return
	}
	time.Sleep(150 * time.Millisecond)
	c.mu.Lock()
	c.isRendering = false
	c.mu.Unlock()
	time.Sleep(150 * time.Millisecond)
}

func (c *Console) render() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		c.mu.Lock()

		var builder strings.Builder

		// If rendering is being stopped, perform one final clear and exit the loop.
		if !c.isRendering {
			if c.lastHeight > 0 {
				builder.WriteString(fmt.Sprintf("\033[%dA\033[J", c.lastHeight))
			}
			c.lastHeight = 0
			fmt.Fprint(os.Stderr, builder.String())
			c.mu.Unlock()
			return
		}

		// Move cursor up to overwrite the previous render.
		if c.lastHeight > 0 {
			builder.WriteString(fmt.Sprintf("\033[%dA", c.lastHeight))
		}
		// Clear from cursor to the end of the screen.
		builder.WriteString("\033[J")

		// Render each task line.
		for _, taskID := range c.taskOrder {
			task, ok := c.tasks[taskID]
			if !ok {
				continue
			}

			sinceLastActivity := time.Since(task.state.lastActivity)
			isInitiallyIdle := task.state.lastActivity.IsZero()

			var sp, tx *color.Color
			var frame string
			var isSpinning bool

			// Determine state based on time since last activity
			if isInitiallyIdle || sinceLastActivity > 10*time.Second {
				// Stale or initially idle
				sp, tx = c.Orange, c.Orange
				isSpinning = false
			} else if sinceLastActivity > 5*time.Second {
				// Idle
				isSpinning = false
				switch task.state.opType {
				case OpDownload:
					sp, tx = c.DarkGreen, c.Yellow
				case OpFeedFetch:
					fallthrough
				default:
					sp, tx = c.Gray, c.Gray
				}
			} else {
				// Active
				isSpinning = true
				switch task.state.opType {
				case OpDownload:
					sp, tx = c.Lime, c.White
				case OpFeedFetch:
					fallthrough
				default:
					sp, tx = c.Cyan, c.White
				}
			}

			if isSpinning {
				frame = task.spinner.next()
			} else {
				frame = task.spinner.current()
			}

			builder.WriteString(fmt.Sprintf("%s %s %s\n", sp.Sprint(frame), c.Bold.Sprint(task.id+":"), tx.Sprint(task.msg)))
		}

		// Write the entire buffer at once to prevent flickering.
		fmt.Fprint(os.Stderr, builder.String())
		c.lastHeight = len(c.taskOrder)
		c.mu.Unlock()
	}
}
