package cmd

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/perpetuallyhorni/tikwm/pkg/client"
	"github.com/perpetuallyhorni/tikwm/pkg/pool"
	"github.com/perpetuallyhorni/tikwm/tools/tikwm/internal/cli"
	cliconfig "github.com/perpetuallyhorni/tikwm/tools/tikwm/internal/config"
	"github.com/spf13/cobra"
)

// TargetManager manages the dynamic processing of targets from a file.
type TargetManager struct {
	cfg       *cliconfig.Config
	appClient *client.Client
	logger    *log.Logger
	console   *cli.Console
	force     bool

	mu               sync.Mutex
	activeTasks      map[string]context.CancelFunc
	watcher          *fsnotify.Watcher
	shutdown         chan struct{}
	wg               sync.WaitGroup
	results          chan string
	reconcileTrigger chan struct{}
}

// downloadCmd represents the 'download' command.
var downloadCmd = &cobra.Command{
	Use:     "download [targets...]",
	Short:   "Download posts or entire user profiles from TikTok (default command).",
	Aliases: []string{"dl"},
	Long: `Downloads posts or entire user profiles from TikTok.
Targets can be usernames or URLs passed as arguments or listed in a targets file.
If using a targets file, the file will be monitored for changes, and workers
will be dynamically reassigned based on the file's content (hot-reloading).
This is the default command if you provide targets without a subcommand.`,
	RunE: runDownload,
}

func runDownload(cmd *cobra.Command, args []string) error {
	targets := getTargets(cfg, console, args)
	isFromFile := len(args) == 0

	if len(targets) == 0 {
		console.Info("No targets specified. Use 'tikwm --help' for more info.")
		return nil
	}

	force, _ := cmd.Flags().GetBool("force")

	// If using targets file and it's configured, run with hot-reloading.
	// Otherwise (e.g. command-line args), run as a one-shot command.
	if isFromFile && cfg.TargetsFile != "" {
		return runDynamicDownload(force)
	}
	return runStaticDownload(force, targets, isFromFile)
}

func runStaticDownload(force bool, targets []string, isFromFile bool) error {
	console.Info("Processing %d target(s) with %d worker(s) in static mode...", len(targets), cfg.MaxWorkers)
	workerPool := pool.New(cfg.MaxWorkers, len(targets))

	for _, targetStr := range targets {
		target := targetStr // Capture for closure
		workerPool.Submit(func() {
			ctx := context.Background()
			parsed := parseTarget(target)
			err := processTargetWithContext(ctx, parsed, appClient, fileLogger, console, force)
			if err != nil {
				// Only log fatal errors in static mode
				if !errors.Is(err, context.Canceled) {
					fileLogger.Printf("ERROR: Failed to process target '%s': %v", target, err)
				}
			} else {
				// Only manage targets file if the source was a file.
				if isFromFile && strings.TrimSpace(target) != "" {
					if err := manageTargetsFile(target, parsed.Type, cfg.TargetsFile, console); err != nil {
						console.Warn("Could not update targets file for '%s': %v", target, err)
					}
				}
			}
		})
	}

	workerPool.Stop()
	console.StopRenderer()
	return nil
}

func runDynamicDownload(force bool) error {
	manager, err := NewTargetManager(force)
	if err != nil {
		return fmt.Errorf("failed to create target manager: %w", err)
	}

	// Handle shutdown on Ctrl+C
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		console.Info("\nShutdown signal received, stopping workers...")
		manager.Stop()
	}()

	return manager.Run()
}

// NewTargetManager creates a new manager for dynamic target processing.
func NewTargetManager(force bool) (*TargetManager, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create file watcher: %w", err)
	}

	return &TargetManager{
		cfg:              cfg,
		appClient:        appClient,
		logger:           fileLogger,
		console:          console,
		force:            force,
		activeTasks:      make(map[string]context.CancelFunc),
		watcher:          watcher,
		shutdown:         make(chan struct{}),
		results:          make(chan string),
		reconcileTrigger: make(chan struct{}, 1),
	}, nil
}

// Run starts the main loop of the TargetManager.
func (tm *TargetManager) Run() error {
	defer tm.watcher.Close()

	// Watch the parent directory of the targets file for robustness.
	targetsDir := filepath.Dir(tm.cfg.TargetsFile)
	if err := os.MkdirAll(targetsDir, 0750); err != nil {
		return fmt.Errorf("could not create targets directory '%s': %w", targetsDir, err)
	}
	if err := tm.watcher.Add(targetsDir); err != nil {
		return fmt.Errorf("could not watch targets directory '%s': %w", targetsDir, err)
	}

	tm.logger.Printf("Starting target manager, watching %s for changes to %s", targetsDir, filepath.Base(tm.cfg.TargetsFile))
	tm.console.Info("Starting dynamic target manager. Watching '%s' for changes.", tm.cfg.TargetsFile)
	tm.console.Info("Top %d targets in the file will be processed in parallel.", tm.cfg.MaxWorkers)
	tm.console.Info("The rest will be queued and processed as workers become available.")
	tm.console.Info("Press Ctrl+C to exit.")

	// Start a goroutine to handle worker completions.
	tm.wg.Add(1)
	go tm.completionHandler()

	// Initial reconciliation.
	tm.triggerReconcile()

	for {
		select {
		case <-tm.reconcileTrigger:
			tm.reconcile()

		case event, ok := <-tm.watcher.Events:
			if !ok {
				return nil // Watcher closed
			}
			// Check if the event is for our specific targets file.
			if filepath.Clean(event.Name) == filepath.Clean(tm.cfg.TargetsFile) {
				// We care about writes, creates, and renames (for atomic saves).
				if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) || event.Has(fsnotify.Rename) {
					tm.logger.Printf("Detected change in targets file: %s", event.String())
					time.Sleep(250 * time.Millisecond) // Debounce
					tm.triggerReconcile()
				}
			}

		case err, ok := <-tm.watcher.Errors:
			if !ok {
				return nil
			}
			tm.logger.Printf("Watcher error: %v", err)
			tm.console.Warn("File watcher error: %v", err)

		case <-tm.shutdown:
			tm.logger.Println("Shutdown signal received by manager event loop.")
			return nil
		}
	}
}

// Stop gracefully shuts down the TargetManager.
func (tm *TargetManager) Stop() {
	tm.mu.Lock()
	// Signal shutdown to the main loop and completion handler.
	close(tm.shutdown)

	// Cancel all active tasks.
	for target, cancel := range tm.activeTasks {
		tm.logger.Printf("Cancelling task for target: %s", target)
		cancel()
	}
	tm.mu.Unlock()

	// Wait for all worker goroutines (processing and completion handler) to finish.
	tm.wg.Wait()
	tm.logger.Println("All manager goroutines finished.")
	tm.console.StopRenderer()
}

// triggerReconcile sends a signal to the reconcile channel if it's not already full.
func (tm *TargetManager) triggerReconcile() {
	select {
	case tm.reconcileTrigger <- struct{}{}:
	default: // A reconcile is already pending.
	}
}

// completionHandler listens for finished workers and triggers reconciliation.
func (tm *TargetManager) completionHandler() {
	defer tm.wg.Done()
	for {
		select {
		case target := <-tm.results:
			tm.mu.Lock()
			// Only trigger reconcile if the task was not externally cancelled
			if _, exists := tm.activeTasks[target]; exists {
				delete(tm.activeTasks, target)
				tm.triggerReconcile()
			}
			tm.mu.Unlock()
		case <-tm.shutdown:
			return
		}
	}
}

// reconcile compares the state of the targets file with the active tasks.
func (tm *TargetManager) reconcile() {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	allTargets := getTargetsFromFile(tm.cfg.TargetsFile, tm.console)
	priorityTargets := make(map[string]struct{})
	for i := 0; i < len(allTargets) && i < tm.cfg.MaxWorkers; i++ {
		priorityTargets[allTargets[i]] = struct{}{}
	}

	// Cancel tasks that are no longer a priority.
	for target, cancel := range tm.activeTasks {
		if _, isPriority := priorityTargets[target]; !isPriority {
			tm.logger.Printf("Target '%s' is no longer a priority target, cancelling.", target)
			tm.console.Warn("Target '%s' removed or de-prioritized. Stopping task.", target)
			cancel()
			delete(tm.activeTasks, target)
			// Remove the task from the console UI immediately for responsiveness.
			tm.console.RemoveTask(client.ExtractUsername(target))
			tm.console.RemoveTask("Post " + client.ExtractUsername(target))
		}
	}

	// Start new tasks for priority targets that are not already running, if slots are available.
	activeCount := len(tm.activeTasks)
	for _, target := range allTargets {
		if activeCount >= tm.cfg.MaxWorkers {
			break // All worker slots are busy.
		}
		if _, isPriority := priorityTargets[target]; !isPriority {
			continue // Not a priority target.
		}
		if _, isActive := tm.activeTasks[target]; !isActive {
			tm.logger.Printf("New priority target '%s', starting task.", target)
			ctx, cancel := context.WithCancel(context.Background())
			tm.activeTasks[target] = cancel
			activeCount++
			tm.wg.Add(1)
			go tm.processTarget(ctx, target)
		}
	}
}

// processTarget is the goroutine function for a single worker.
func (tm *TargetManager) processTarget(ctx context.Context, target string) {
	defer tm.wg.Done()

	parsed := parseTarget(target)
	err := processTargetWithContext(ctx, parsed, tm.appClient, tm.logger, tm.console, tm.force)

	// On successful completion (err is nil), manage the targets file.
	if err == nil {
		tm.console.Success("Target '%s' finished processing.", target)
		if strings.TrimSpace(target) != "" {
			tm.updateTargetsFileOnSuccess(target, parsed.Type)
		}
	} else if !errors.Is(err, context.Canceled) {
		// Log errors, but not cancellations.
		tm.console.Error("Target '%s' finished with an error.", target)
		tm.logger.Printf("ERROR processing target %s: %v", target, err)
	}

	// Signal completion to the manager regardless of outcome.
	select {
	case tm.results <- target:
	case <-tm.shutdown:
	}
}

// updateTargetsFileOnSuccess handles the file modification logic safely.
func (tm *TargetManager) updateTargetsFileOnSuccess(target, targetType string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	// Writing to the file will trigger the watcher. This is expected and desired.
	if err := manageTargetsFile(target, targetType, tm.cfg.TargetsFile, tm.console); err != nil {
		tm.console.Warn("Could not update targets file for '%s': %v", target, err)
		tm.logger.Printf("WARN: Could not update targets file for '%s': %v", target, err)
	}
}
