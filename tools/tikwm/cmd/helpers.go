package cmd

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/adrg/xdg"
	"github.com/perpetuallyhorni/tikwm/pkg/client"
	"github.com/perpetuallyhorni/tikwm/pkg/config"
	"github.com/perpetuallyhorni/tikwm/pkg/logging"
	"github.com/perpetuallyhorni/tikwm/tools/tikwm/internal/cli"
	"github.com/spf13/cobra"
)

// ParsedTarget represents a parsed target, which can be either a user or a post.
type ParsedTarget struct {
	Type  string // "user" or "post"
	Value string // original string
}

// applyFlagOverrides applies command-line flag overrides to the configuration.
func applyFlagOverrides(cmd *cobra.Command, cfg *config.Config) {
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
}

// getTargets retrieves targets from command-line arguments or a targets file.
func getTargets(cfg *config.Config, console *cli.Console, args []string) []string {
	if len(args) > 0 {
		return args
	}
	if cfg.TargetsFile == "" {
		return nil
	}
	file, err := os.Open(cfg.TargetsFile)
	if err != nil {
		console.Warn("Could not open targets file '%s': %v", cfg.TargetsFile, err)
		return nil
	}
	defer func() {
		if err := file.Close(); err != nil {
			console.Warn("Could not close targets file '%s': %v", cfg.TargetsFile, err)
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
		console.Warn("Error reading targets file '%s': %v", cfg.TargetsFile, err)
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

// processTarget processes a single target, either downloading a post or a user's profile.
func processTarget(target ParsedTarget, appClient *client.Client, logger *log.Logger, console *cli.Console, cfg *config.Config, force bool) error {
	switch target.Type {
	case "post":
		console.Info("Processing post: %s", target.Value)
		err := appClient.DownloadPost(target.Value, force, logger)
		if err == nil {
			console.Success("Successfully processed post %s", target.Value)
		}
		return err
	case "user":
		username := client.ExtractUsername(target.Value)
		progressCb := func(current, total int, msg string) {
			if total > 0 {
				progressMsg := fmt.Sprintf("[%s] Processing %d/%d: %s", console.Bold.Sprint(username), current, total, msg)
				console.UpdateProgress(progressMsg)
			} else {
				progressMsg := fmt.Sprintf("[%s] %s", console.Bold.Sprint(username), msg)
				console.UpdateProgress(progressMsg)
			}
		}
		console.StartProgress(fmt.Sprintf("[%s] Preparing to fetch...", console.Bold.Sprint(username)))
		err := appClient.DownloadProfile(username, force, logger, progressCb)
		console.StopProgress()
		if err == nil {
			console.Success("Successfully processed user %s", username)
		}
		return err
	default:
		return fmt.Errorf("unknown target type for '%s'", target.Value)
	}
}

// setupFileLogger sets up a file logger to log application events.
func setupFileLogger(clean bool, targets []string, cfg *config.Config) (*log.Logger, error) {
	logPath, err := xdg.StateFile(filepath.Join(config.AppName, "app.log"))
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
		return nil
	}

	var newLines []string
	switch targetType {
	case "post":
		console.Info("Commenting out downloaded post in targets file.")
		lines[targetIdx] = "# " + lines[targetIdx]
		newLines = lines
	case "user":
		console.Info("Moving processed user to the end of targets file.")
		userLine := lines[targetIdx]
		tempLines := append(lines[:targetIdx], lines[targetIdx+1:]...)
		// Remove all trailing empty lines from tempLines
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
