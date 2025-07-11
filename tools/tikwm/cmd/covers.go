package cmd

import (
	"fmt"

	"github.com/perpetuallyhorni/tikwm/pkg/client"
	"github.com/perpetuallyhorni/tikwm/pkg/pool"
	"github.com/perpetuallyhorni/tikwm/tools/tikwm/internal/cli"
	"github.com/spf13/cobra"
)

// coversCmd represents the covers command.
var coversCmd = &cobra.Command{
	Use:   "covers [targets...]",
	Short: "Download missing cover images for users.",
	RunE: func(cmd *cobra.Command, args []string) error {
		targets := getTargets(cfg, console, args)
		if len(targets) == 0 {
			console.Info("No targets specified for cover download.")
			return nil
		}

		workerPool := pool.New(cfg.MaxWorkers, len(targets))

		for _, target := range targets {
			username := client.ExtractUsername(target) // Capture for closure
			workerPool.Submit(func() {
				console.AddTask(username, "Checking for missing covers...", cli.OpFeedFetch)
				progressCb := func(current, total int, msg string) {
					console.UpdateTaskActivity(username)
					if total > 0 {
						console.UpdateTaskMessage(username, fmt.Sprintf("%d/%d: %s", current, total, msg))
					} else {
						console.UpdateTaskMessage(username, msg)
					}
				}

				err := appClient.DownloadCoversForUser(username, fileLogger, progressCb)
				console.RemoveTask(username)

				if err != nil {
					console.Error("Failed to download covers for %s: %v", username, err)
				} else {
					console.Success("Finished cover check for %s.", username)
				}
			})
		}

		workerPool.Stop()
		console.StopRenderer()
		return nil
	},
}
