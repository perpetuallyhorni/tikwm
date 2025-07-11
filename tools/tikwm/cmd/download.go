package cmd

import (
	"strings"

	"github.com/perpetuallyhorni/tikwm/pkg/pool"
	"github.com/spf13/cobra"
)

// downloadCmd represents the 'download' command.
var downloadCmd = &cobra.Command{
	Use:     "download [targets...]",                                                 // Usage string for the command.
	Short:   "Download posts or entire user profiles from TikTok (default command).", // Short description of the command.
	Aliases: []string{"dl"},                                                          // Aliases for the command.
	Long: `Downloads posts or entire user profiles from TikTok.
Targets can be usernames or URLs passed as arguments or listed in a targets file.
This is the default command if you provide targets without a subcommand.`, // Long description of the command.
	RunE: runDownload, // Function to execute when the command is run.
}

// runDownload contains the core logic for downloading targets.
func runDownload(cmd *cobra.Command, args []string) error {
	targets := getTargets(cfg, console, args)
	isFromFile := len(args) == 0
	if len(targets) == 0 {
		console.Info("No targets specified. Use 'tikwm --help' for more info.")
		return nil
	}

	force, _ := cmd.Flags().GetBool("force")
	console.Info("Processing %d target(s) with %d worker(s)...", len(targets), cfg.MaxWorkers)
	workerPool := pool.New(cfg.MaxWorkers, len(targets))

	for _, targetStr := range targets {
		target := targetStr // Capture for closure
		workerPool.Submit(func() {
			parsed := parseTarget(target)
			err := processTarget(parsed, appClient, fileLogger, console, force)
			if err != nil {
				fileLogger.Printf("ERROR: Failed to process target '%s': %v", target, err)
			} else {
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
