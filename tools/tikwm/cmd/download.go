package cmd

import (
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
// It is used by both the 'download' command and the root command as its default action.
func runDownload(cmd *cobra.Command, args []string) error {
	targets := getTargets(cfg, console, args) // Get the list of targets from arguments or file.
	isFromFile := len(args) == 0              // Check if targets are read from file.
	if len(targets) == 0 {
		console.Info("No targets specified. Use 'tikwm --help' for more info.") // Inform the user if no targets are specified.
		return nil
	}

	force, _ := cmd.Flags().GetBool("force") // Get the value of the 'force' flag.

	// Iterate over each target.
	for _, targetStr := range targets {
		parsed := parseTarget(targetStr) // Parse the target string.
		// Process the target.
		err := processTarget(parsed, appClient, fileLogger, console, cfg, force)
		if err != nil {
			console.Error("Error processing target '%s': %v", targetStr, err) // Log an error if processing fails.
		} else {
			// If the target was read from a file, update the targets file.
			if isFromFile {
				if err := manageTargetsFile(targetStr, parsed.Type, cfg.TargetsFile, console); err != nil {
					console.Warn("Could not update targets file: %v", err) // Log a warning if updating the targets file fails.
				}
			}
		}
	}
	return nil
}

func init() {
	// Flags are defined as persistent on the root command to be available here.
}
