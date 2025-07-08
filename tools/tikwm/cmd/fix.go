package cmd

import (
	"fmt"

	"github.com/perpetuallyhorni/tikwm/pkg/client"
	"github.com/spf13/cobra"
)

// fixCmd represents the fix command.
var fixCmd = &cobra.Command{
	Use:   "fix [targets...]",
	Short: "Download videos that are in the database but missing the hash for the specified quality(es).",
	Long: `Checks the database for posts belonging to the specified users (targets)
and downloads any videos that are missing the qualities specified in your config or via the --quality flag.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get the targets to fix from the arguments or the config file.
		targets := getTargets(cfg, console, args)
		// If no targets were specified, print a message and return.
		if len(targets) == 0 {
			console.Info("No targets specified to fix.")
			return nil
		}

		// Print a message indicating the quality setting being used.
		console.Info("Fixing missing videos with quality setting: %s", console.Bold.Sprint(cfg.Quality))

		// Iterate over the targets.
		for _, target := range targets {
			// Extract the username from the target.
			username := client.ExtractUsername(target)
			// Define a callback function to update the progress of the fix process.
			progressCb := func(current, total int, msg string) {
				var progressMsg string
				if total > 0 {
					progressMsg = fmt.Sprintf("[%s] Fixing %d/%d: %s", console.Bold.Sprint(username), current, total, msg)
				} else {
					progressMsg = fmt.Sprintf("[%s] %s", console.Bold.Sprint(username), msg)
				}
				console.UpdateProgress(progressMsg)
			}

			// Start the progress indicator.
			console.StartProgress(fmt.Sprintf("[%s] Starting fix process...", console.Bold.Sprint(username)))
			// Fix the missing videos for the target.
			err := appClient.FixProfile(username, fileLogger, progressCb)
			// Stop the progress indicator.
			console.StopProgress()
			// If there was an error fixing the videos, print an error message.
			if err != nil {
				console.Error("Failed to fix missing videos for %s: %v", username, err)
			} else {
				// Otherwise, print a success message.
				console.Success("Finished fixing missing videos for %s.", username)
			}
		}

		// Return nil to indicate success.
		return nil
	},
}
