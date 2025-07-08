package cmd

import (
	"encoding/json"
	"fmt"

	tikwm "github.com/perpetuallyhorni/tikwm/internal"
	"github.com/perpetuallyhorni/tikwm/pkg/client"
	"github.com/spf13/cobra"
)

// infoCmd represents the info command.
var infoCmd = &cobra.Command{
	Use:   "info [targets...]",
	Short: "Print info about user profiles.",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Iterate over the provided target usernames or URLs.
		for _, target := range args {
			// Extract the username from the target.
			username := client.ExtractUsername(target)
			// Print a message indicating that we are fetching information for the user.
			console.Info("Fetching info for %s...", username)
			// Get the user details.
			info, err := tikwm.GetUserDetail(username)
			// If there is an error getting user details, return an error.
			if err != nil {
				return fmt.Errorf("failed to get user details for %s: %w", username, err)
			}
			// Marshal the user info into a JSON string with indentation.
			buffer, err := json.MarshalIndent(info, "", "  ")
			// If there is an error marshalling the user info, return an error.
			if err != nil {
				return fmt.Errorf("failed to marshal user info: %w", err)
			}
			// Print the JSON string.
			fmt.Println(string(buffer))
		}
		return nil
	},
}
