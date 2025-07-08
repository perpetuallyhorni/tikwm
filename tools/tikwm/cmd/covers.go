package cmd

import (
	"fmt"

	"github.com/perpetuallyhorni/tikwm/pkg/client"
	"github.com/spf13/cobra"
)

// coversCmd represents the covers command.
var coversCmd = &cobra.Command{
	Use:   "covers [targets...]",
	Short: "Download missing cover images for users.",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get the targets (usernames) from the configuration or command line arguments.
		targets := getTargets(cfg, console, args)

		// If no targets are specified, print a message and return.
		if len(targets) == 0 {
			console.Info("No targets specified for cover download.")
			return nil
		}

		// Iterate over each target.
		for _, target := range targets {
			// Extract the username from the target.
			username := client.ExtractUsername(target)

			// Define a callback function to update the progress of the download.
			progressCb := func(current, total int, msg string) {
				progressMsg := fmt.Sprintf("[%s] Downloading covers %d/%d: %s", console.Bold.Sprint(username), current, total, msg)
				console.UpdateProgress(progressMsg)
			}

			// Start the progress indicator.
			console.StartProgress(fmt.Sprintf("[%s] Checking for missing covers...", console.Bold.Sprint(username)))

			// Download the covers for the user.
			err := appClient.DownloadCoversForUser(username, fileLogger, progressCb)

			// Stop the progress indicator.
			console.StopProgress()

			// If an error occurred during the download, print an error message.
			if err != nil {
				console.Error("Failed to download covers for %s: %v", username, err)
			} else {
				// If the download was successful, print a success message.
				console.Success("Finished cover check for %s.", username)
			}
		}
		return nil
	},
}
