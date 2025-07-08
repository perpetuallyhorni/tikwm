package cmd

import (
	"fmt"

	tikwm "github.com/perpetuallyhorni/tikwm/internal"
	"github.com/perpetuallyhorni/tikwm/pkg/client"
	"github.com/spf13/cobra"
)

// debugCmd represents the base command for debugging tools.
var debugCmd = &cobra.Command{
	Use:   "debug",
	Short: "Debugging tools for tikwm.",
	RunE: func(cmd *cobra.Command, args []string) error {
		// If no subcommand is specified, print the help message.
		return cmd.Help()
	},
}

// debugFeedCmd represents the command to dump the raw JSON response of a user's feed.
var debugFeedCmd = &cobra.Command{
	Use:   "feed [username]",
	Short: "Dump the raw JSON response of the first page of a user's feed.",
	Long: `This command is for debugging. It fetches the first page of a user's feed 
(user/posts endpoint) and prints the raw, unparsed JSON response directly to stdout.
This is useful for inspecting the data structure returned by the API.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Extract the username from the arguments.
		username := client.ExtractUsername(args[0])
		console.Info("Fetching raw feed for %s...", username)

		// We use the raw method to get the bytes without any parsing or error handling on the content.
		// We call the 'user/posts' method with a count of 5 to get a small, representative sample.
		query := map[string]string{"unique_id": username, "count": "5", "cursor": "0"}
		responseBytes, err := tikwm.Raw("user/posts", query)
		if err != nil {
			return fmt.Errorf("failed to get raw feed for %s: %w", username, err)
		}

		// Print the raw bytes to stdout.
		fmt.Println(string(responseBytes))
		return nil
	},
}

// init initializes the debug command and its subcommands.
func init() {
	debugCmd.AddCommand(debugFeedCmd)
}
