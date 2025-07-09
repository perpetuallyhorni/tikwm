package cmd

import (
	"github.com/perpetuallyhorni/tikwm/tools/tikwm/internal/update"
	"github.com/spf13/cobra"
)

// updateCmd represents the update command.
var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update tikwm to the latest version.",
	Long: `Checks for the latest version of tikwm on GitHub and, if a newer version is found,
downloads and installs it.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// ApplyUpdate now contains all necessary logic, including checking if already latest.
		return update.ApplyUpdate(console, version)
	},
}
