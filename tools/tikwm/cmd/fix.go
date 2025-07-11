package cmd

import (
	"fmt"

	"github.com/perpetuallyhorni/tikwm/pkg/client"
	"github.com/perpetuallyhorni/tikwm/pkg/pool"
	"github.com/perpetuallyhorni/tikwm/tools/tikwm/internal/cli"
	"github.com/spf13/cobra"
)

// fixCmd represents the fix command.
var fixCmd = &cobra.Command{
	Use:   "fix [targets...]",
	Short: "Download videos that are in the database but missing the hash for the specified quality(es).",
	Long: `Checks the database for posts belonging to the specified users (targets)
and downloads any videos that are missing the qualities specified in your config or via the --quality flag.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		targets := getTargets(cfg, console, args)
		if len(targets) == 0 {
			console.Info("No targets specified to fix.")
			return nil
		}

		console.Info("Fixing missing videos with quality setting: %s", console.Bold.Sprint(cfg.Quality))
		workerPool := pool.New(cfg.MaxWorkers, len(targets))

		for _, target := range targets {
			username := client.ExtractUsername(target) // Capture for closure
			workerPool.Submit(func() {
				console.AddTask(username, "Starting fix...", cli.OpFeedFetch)
				progressCb := func(current, total int, msg string) {
					console.UpdateTaskActivity(username)
					if total > 0 {
						console.UpdateTaskMessage(username, fmt.Sprintf("%d/%d: %s", current, total, msg))
					} else {
						console.UpdateTaskMessage(username, msg)
					}
				}

				err := appClient.FixProfile(username, fileLogger, progressCb)
				console.RemoveTask(username)

				if err != nil {
					console.Error("Failed to fix missing videos for %s: %v", username, err)
				} else {
					console.Success("Finished fixing missing videos for %s.", username)
				}
			})
		}

		workerPool.Stop()
		console.StopRenderer()
		return nil
	},
}
