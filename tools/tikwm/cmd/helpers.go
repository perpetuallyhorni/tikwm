package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/adrg/xdg"
	tikwm "github.com/perpetuallyhorni/tikwm/internal"
	"github.com/perpetuallyhorni/tikwm/pkg/client"
	"github.com/perpetuallyhorni/tikwm/pkg/logging"
	"github.com/perpetuallyhorni/tikwm/tools/tikwm/internal/cli"
	cliconfig "github.com/perpetuallyhorni/tikwm/tools/tikwm/internal/config"
	"github.com/spf13/cobra"
)

// ParsedTarget represents a parsed target, which can be either a user or a post.
type ParsedTarget struct {
	Type  string // "user" or "post"
	Value string // original string
}

// applyFlagOverrides applies command-line flag overrides to the configuration.
func applyFlagOverrides(cmd *cobra.Command, cfg *cliconfig.Config) {
	if cmd.Flag("dir").Changed {
		cfg.DownloadPath, _ = cmd.Flags().GetString("dir")
	}
	if cmd.Flag("targets").Changed {
		cfg.TargetsFile, _ = cmd.Flags().GetString("targets")
	}
	if cmd.Flag("since").Changed {
		cfg.Since, _ = cmd.Flags().GetString("since")
	}
	if cmd.Flag("quality").Changed {
		cfg.Quality, _ = cmd.Flags().GetString("quality")
	}
	if cmd.Flag("workers").Changed {
		if val, _ := cmd.Flags().GetInt("workers"); val > 0 {
			cfg.MaxWorkers = val
		}
	}
	if cmd.Flag("retry-on-429").Changed {
		cfg.RetryOn429, _ = cmd.Flags().GetBool("retry-on-429")
	}
	if cmd.Flag("download-covers").Changed {
		cfg.DownloadCovers, _ = cmd.Flags().GetBool("download-covers")
	}
	if cmd.Flag("cover-type").Changed {
		cfg.CoverType, _ = cmd.Flags().GetString("cover-type")
	}
	if cmd.Flag("download-avatars").Changed {
		cfg.DownloadAvatars, _ = cmd.Flags().GetBool("download-avatars")
	}
	if cmd.Flag("save-post-title").Changed {
		cfg.SavePostTitle, _ = cmd.Flags().GetBool("save-post-title")
	}
	if cmd.Flag("feed-cache").Changed {
		cfg.FeedCache, _ = cmd.Flags().GetBool("feed-cache")
	}
	if cmd.Flag("feed-cache-ttl").Changed {
		cfg.FeedCacheTTL, _ = cmd.Flags().GetString("feed-cache-ttl")
	}
}

// getTargets retrieves targets from command-line arguments or a targets file.
func getTargets(cfg *cliconfig.Config, console *cli.Console, args []string) []string {
	if len(args) > 0 {
		return args
	}
	return getTargetsFromFile(cfg.TargetsFile, console)
}

// getTargetsFromFile reads targets from the specified file.
func getTargetsFromFile(filePath string, console *cli.Console) []string {
	if filePath == "" {
		return nil
	}
	file, err := os.Open(filePath) // #nosec G304
	if err != nil {
		// Don't warn if it just doesn't exist, as it may be created later.
		if !os.IsNotExist(err) {
			console.Warn("Could not open targets file '%s': %v", filePath, err)
		}
		return nil
	}
	defer func() {
		if err := file.Close(); err != nil {
			console.Warn("Could not close targets file '%s': %v", filePath, err)
		}
	}()

	var fileTargets []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			fileTargets = append(fileTargets, line)
		}
	}
	if err := scanner.Err(); err != nil {
		console.Warn("Error reading targets file '%s': %v", filePath, err)
	}
	return fileTargets
}

// parseTarget parses a target string and determines its type (user or post).
func parseTarget(target string) ParsedTarget {
	trimmedTarget := strings.TrimSpace(target)
	if strings.Contains(trimmedTarget, "tiktok.com") && strings.Contains(trimmedTarget, "/video/") {
		return ParsedTarget{Type: "post", Value: trimmedTarget}
	}
	if u, err := url.Parse(trimmedTarget); err == nil && (u.Scheme == "http" || u.Scheme == "https") && strings.Contains(u.Host, "tiktok.com") {
		return ParsedTarget{Type: "user", Value: trimmedTarget}
	}
	return ParsedTarget{Type: "user", Value: trimmedTarget}
}

// processTargetWithContext processes a single target, either downloading a post or a user's profile.
func processTargetWithContext(ctx context.Context, target ParsedTarget, appClient *client.Client, logger *log.Logger, console *cli.Console, force bool) error {
	var taskID string
	var err error

	switch target.Type {
	case "post":
		taskID = "Post " + client.ExtractUsername(target.Value)
		console.AddTask(taskID, "Downloading...", cli.OpDownload)
		console.UpdateTaskActivity(taskID)
		err = appClient.DownloadPost(ctx, target.Value, force, logger)

	case "user":
		username := client.ExtractUsername(target.Value)
		taskID = username
		console.AddTask(taskID, "Preparing to fetch...", cli.OpFeedFetch)

		progressCb := func(current, total int, msg string) {
			console.UpdateTaskActivity(taskID)
			console.UpdateTaskMessage(taskID, fmt.Sprintf("Processing %d/%d: %s", current, total, msg))
		}
		err = appClient.DownloadProfile(ctx, username, force, logger, progressCb)

	default:
		err = fmt.Errorf("unknown target type for '%s'", target.Value)
	}

	console.RemoveTask(taskID)

	if err != nil {
		// Don't log cancellation as a failure, it's expected.
		if errors.Is(err, context.Canceled) {
			logger.Printf("Task for '%s' was cancelled.", target.Value)
			return err
		}
		if errors.Is(err, tikwm.ErrDiskSpace) {
			console.Error("Disk space error processing '%s'. Halting.", target.Value)
			return err // Propagate fatal error
		}
		console.Error("Failed to process target '%s': %v", target.Value, err)
		// Log the error, but return nil so other workers in a static pool can continue.
		// The dynamic manager will handle the returned error differently.
		return err
	}

	return nil
}

// setupFileLogger sets up a file logger to log application events.
func setupFileLogger(clean bool, targets []string, cfg *cliconfig.Config) (*log.Logger, error) {
	logPath, err := xdg.StateFile(filepath.Join(cliconfig.AppName, "app.log"))
	if err != nil {
		return nil, fmt.Errorf("could not get log file path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(logPath), 0750); err != nil {
		return nil, fmt.Errorf("could not create log directory: %w", err)
	}

	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0640) // #nosec G304 G302
	if err != nil {
		return nil, fmt.Errorf("could not open log file: %w", err)
	}

	var writer io.Writer = f
	if clean {
		writer = logging.NewRedactingWriter(f, cfg.DownloadPath, targets)
	}

	return log.New(writer, "", log.LstdFlags), nil
}

// manageTargetsFile manages the targets file by commenting out processed posts or moving processed users.
func manageTargetsFile(targetLine, targetType, filePath string, console *cli.Console) error {
	input, err := os.ReadFile(filePath) // #nosec G304
	if err != nil {
		return fmt.Errorf("could not read targets file: %w", err)
	}

	lines := strings.Split(string(input), "\n")
	targetIdx := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == strings.TrimSpace(targetLine) {
			targetIdx = i
			break
		}
	}
	if targetIdx == -1 {
		console.Info("Could not find target '%s' in targets file to update its status (file may have changed).", targetLine)
		return nil
	}

	var newLines []string
	switch targetType {
	case "post":
		lines[targetIdx] = "# " + lines[targetIdx]
		newLines = lines
	case "user":
		userLine := lines[targetIdx]
		tempLines := append(lines[:targetIdx], lines[targetIdx+1:]...)
		for len(tempLines) > 0 && strings.TrimSpace(tempLines[len(tempLines)-1]) == "" {
			tempLines = tempLines[:len(tempLines)-1]
		}
		newLines = append(tempLines, userLine)
	}

	var finalLines []string
	for _, line := range newLines {
		if strings.TrimSpace(line) != "" || (len(finalLines) > 0 && strings.TrimSpace(finalLines[len(finalLines)-1]) != "") {
			finalLines = append(finalLines, line)
		}
	}

	output := strings.Join(finalLines, "\n")
	if len(finalLines) > 0 && !strings.HasSuffix(output, "\n") {
		output += "\n"
	}
	return os.WriteFile(filePath, []byte(output), 0640) // #nosec G306
}
