package cmd

import (
	"bufio"
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

const markerComment = "# Completed targets are moved below this line. New targets should be added above."

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

	if isFromFile && cfg.TargetsFile != "" && cfg.DaemonMode {
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
	defer func() {
		if err := tm.watcher.Close(); err != nil {
			tm.logger.Printf("Error closing watcher: %v", err)
		}
	}()
	targetsDir := filepath.Dir(tm.cfg.TargetsFile)
	if err := os.MkdirAll(targetsDir, 0750); err != nil {
		return fmt.Errorf("could not create targets directory '%s': %w", targetsDir, err)
	}
	if err := tm.watcher.Add(targetsDir); err != nil {
		return fmt.Errorf("could not watch targets directory '%s': %w", targetsDir, err)
	}
	if err := tm.initializeTargetsFileState(); err != nil {
		return fmt.Errorf("failed to initialize targets file state: %w", err)
	}
	tm.logger.Printf("Starting target manager, watching %s for changes to %s", targetsDir, filepath.Base(tm.cfg.TargetsFile))
	tm.console.Info("Starting daemon mode. Watching '%s' for changes.", tm.cfg.TargetsFile)
	tm.console.Info("Press Ctrl+C to exit.")
	tm.wg.Add(1)
	go tm.completionHandler()
	tm.triggerReconcile()
	for {
		select {
		case <-tm.reconcileTrigger:
			tm.reconcile()
		case event, ok := <-tm.watcher.Events:
			if !ok {
				return nil
			}
			if filepath.Clean(event.Name) == filepath.Clean(tm.cfg.TargetsFile) {
				if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) || event.Has(fsnotify.Rename) {
					tm.logger.Printf("Detected change in targets file: %s", event.String())
					time.Sleep(250 * time.Millisecond)
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
	close(tm.shutdown)
	for target, cancel := range tm.activeTasks {
		tm.logger.Printf("Cancelling task for target: %s", target)
		cancel()
	}
	tm.mu.Unlock()
	tm.wg.Wait()
	tm.logger.Println("All manager goroutines finished.")
	tm.console.StopRenderer()
}

// triggerReconcile sends a signal to the reconcile channel if it's not already full.
func (tm *TargetManager) triggerReconcile() {
	select {
	case tm.reconcileTrigger <- struct{}{}:
	default:
	}
}

// completionHandler listens for finished workers and triggers reconciliation.
func (tm *TargetManager) completionHandler() {
	defer tm.wg.Done()
	for {
		select {
		case target := <-tm.results:
			tm.mu.Lock()
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

	priorityTargets, err := getPriorityTargetsFromFile(tm.cfg.TargetsFile, tm.console)
	if err != nil {
		tm.console.Error("Failed to read targets file: %v", err)
		return
	}
	prioritySet := make(map[string]struct{})
	for _, target := range priorityTargets {
		prioritySet[target] = struct{}{}
	}

	for target, cancel := range tm.activeTasks {
		if _, isPriority := prioritySet[target]; !isPriority {
			tm.logger.Printf("Target '%s' is no longer a priority, cancelling.", target)
			tm.console.Warn("Target '%s' processed or de-prioritized. Stopping task.", target)
			cancel()
			delete(tm.activeTasks, target)
			tm.console.RemoveTask(client.ExtractUsername(target))
			tm.console.RemoveTask("Post " + client.ExtractUsername(target))
		}
	}

	if len(priorityTargets) == 0 && len(tm.activeTasks) == 0 {
		tm.enterDaemonPoll()
		return
	}

	activeCount := len(tm.activeTasks)
	for _, target := range priorityTargets {
		if activeCount >= tm.cfg.MaxWorkers {
			break
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

func (tm *TargetManager) enterDaemonPoll() {
	pollInterval, err := time.ParseDuration(tm.cfg.DaemonPollInterval)
	if err != nil {
		pollInterval = 60 * time.Second
		tm.console.Warn("Invalid daemon_poll_interval '%s', using default 60s. Error: %v", tm.cfg.DaemonPollInterval, err)
	}

	tm.console.Info("All targets processed. Entering low-frequency poll mode (checking every %s).", pollInterval)

	go func() {
		select {
		case <-time.After(pollInterval):
			if err := tm.initializeTargetsFileState(); err != nil {
				tm.console.Error("Failed to reset targets file for new poll cycle: %v", err)
			}
			tm.triggerReconcile()
		case <-tm.shutdown:
			return
		}
	}()
}

// processTarget is the goroutine function for a single worker.
func (tm *TargetManager) processTarget(ctx context.Context, target string) {
	defer tm.wg.Done()

	parsed := parseTarget(target)
	err := processTargetWithContext(ctx, parsed, tm.appClient, tm.logger, tm.console, tm.force)

	if err == nil {
		tm.console.Success("Target '%s' finished processing.", target)
		if strings.TrimSpace(target) != "" {
			tm.updateTargetsFileOnSuccess(target)
		}
	} else if !errors.Is(err, context.Canceled) {
		tm.console.Error("Target '%s' finished with an error.", target)
		tm.logger.Printf("ERROR processing target %s: %v", target, err)
	}

	select {
	case tm.results <- target:
	case <-tm.shutdown:
	}
}

// updateTargetsFileOnSuccess moves a successfully processed user below the marker.
func (tm *TargetManager) updateTargetsFileOnSuccess(target string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	lines, err := readLines(tm.cfg.TargetsFile)
	if err != nil {
		tm.console.Warn("Could not update targets file for '%s': %v", target, err)
		return
	}
	var newLines, completedLines []string
	var found bool
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == target {
			completedLines = append(completedLines, line)
			found = true
		} else if trimmed != markerComment {
			newLines = append(newLines, line)
		}
	}

	if found {
		finalContent := strings.Join(newLines, "\n") + "\n" + markerComment + "\n" + strings.Join(completedLines, "\n")
		// #nosec G306
		if err := os.WriteFile(tm.cfg.TargetsFile, []byte(finalContent), 0640); err != nil {
			tm.console.Warn("Failed to write updated targets file: %v", err)
		}
	}
}

// initializeTargetsFileState ensures the marker is at the end of the file.
func (tm *TargetManager) initializeTargetsFileState() error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	lines, err := readLines(tm.cfg.TargetsFile)
	if err != nil {
		if os.IsNotExist(err) {
			return os.WriteFile(tm.cfg.TargetsFile, []byte(markerComment+"\n"), 0640) // #nosec G306
		}
		return err
	}

	var regularLines []string
	for _, line := range lines {
		if strings.TrimSpace(line) != markerComment {
			regularLines = append(regularLines, line)
		}
	}

	content := strings.Join(regularLines, "\n")
	if len(regularLines) > 0 {
		content += "\n"
	}
	content += markerComment + "\n"

	// #nosec G306
	return os.WriteFile(tm.cfg.TargetsFile, []byte(content), 0640)
}

// getPriorityTargetsFromFile reads targets from the specified file up to the marker.
func getPriorityTargetsFromFile(filePath string, console *cli.Console) ([]string, error) {
	lines, err := readLines(filePath)
	if err != nil {
		return nil, err
	}
	var fileTargets []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == markerComment {
			break
		}
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			fileTargets = append(fileTargets, trimmed)
		}
	}
	return fileTargets, nil
}

// readLines is a helper to read a file into a slice of strings.
func readLines(filePath string) ([]string, error) {
	file, err := os.Open(filePath) // #nosec G304
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := file.Close(); err != nil {
			log.Printf("Error closing file: %v", err)
		}
	}()
	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, scanner.Err()
}
