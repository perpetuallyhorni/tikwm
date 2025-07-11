package cmd

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"

	tikwm "github.com/perpetuallyhorni/tikwm/internal"
	"github.com/perpetuallyhorni/tikwm/pkg/client"
	"github.com/perpetuallyhorni/tikwm/pkg/network"
	"github.com/perpetuallyhorni/tikwm/pkg/storage/sqlite"
	"github.com/perpetuallyhorni/tikwm/tools/tikwm/internal/cli"
	cliconfig "github.com/perpetuallyhorni/tikwm/tools/tikwm/internal/config"
	"github.com/perpetuallyhorni/tikwm/tools/tikwm/internal/update"
	"github.com/spf13/cobra"
)

var (
	// cfg stores the application configuration.
	cfg *cliconfig.Config
	// appClient is the client used to interact with the TikTok API.
	appClient *client.Client
	// console is the CLI console for output.
	console *cli.Console
	// fileLogger is the logger for writing logs to a file.
	fileLogger *log.Logger
	// database is the storage interface for storing data.
	database *sqlite.DB
	// flagConfigPath is the path to the config file.
	flagConfigPath string
	// flagQuiet enables or disables quiet mode.
	flagQuiet bool
	// version is the version of the application. It is set at build time.
	// See the .goreleaser.yml file for more information.
	version string
)

// SetVersion sets the version of the application.
func SetVersion(v string) {
	version = v
	if rootCmd != nil {
		rootCmd.Version = v
	}
}

var rootCmd = &cobra.Command{
	Use:   "tikwm [command|targets...]",
	Short: "A downloader for TikTok, powered by tikwm.com.",
	Long: `A downloader for TikTok, powered by tikwm.com.

Run 'tikwm [targets...]' to download content or use a specific command.
For example:
  tikwm some_user_name --quality hd
  tikwm download https://www.tiktok.com/@some_user_name/video/12345
  tikwm fix some_user_name`,
	Args: cobra.ArbitraryArgs,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Do not run hooks for completion, edit, or debug commands
		isLightweightCmd := false
		lightweightCommands := []string{"completion", "edit", "debug", "update"}
		for c := cmd; c != nil; c = c.Parent() {
			for _, lwCmd := range lightweightCommands {
				if c.Name() == lwCmd {
					isLightweightCmd = true
					break
				}
			}
			if isLightweightCmd {
				break
			}
		}

		// Return early for completion command.
		if cmd.Name() == "completion" {
			return nil
		}

		// The full setup for commands that need it.
		if !isLightweightCmd {
			// Initialize the network manager with IP rotation.
			if err := network.InitManager(cfg.BindAddress); err != nil {
				return err
			}

			targets := getTargets(cfg, console, args)
			// Check the flag to clean logs or not.
			cleanLogs, _ := cmd.Flags().GetBool("clean-logs")

			var err error
			// Setup the file logger
			fileLogger, err = setupFileLogger(cleanLogs, targets, cfg)
			if err != nil {
				return fmt.Errorf("failed to set up file logger: %w", err)
			}

			// If debug is enabled, write to both file and stderr.
			if val, _ := cmd.Flags().GetBool("debug"); val {
				mw := io.MultiWriter(fileLogger.Writer(), os.Stderr)
				fileLogger.SetOutput(mw)
			}

			// Initialize the global rate limiter.
			tikwm.InitRateLimiter(context.Background())

			// Initialize the database.
			database, err = sqlite.New(cfg.DatabasePath)
			if err != nil {
				return fmt.Errorf("error initializing database: %w", err)
			}

			// Create a new client, passing the database which satisfies the storage.Storer interface.
			appClient, err = client.New(&cfg.Config, database, fileLogger)
			if err != nil {
				return fmt.Errorf("error creating client: %w", err)
			}
		}

		// Update Check runs for commands that did the full setup.
		if !isLightweightCmd && cfg.CheckForUpdates {
			latestVersion, err := update.CheckForUpdate(version)
			if err != nil {
				// Non-fatal, just warn the user.
				console.Warn("Update check failed: %v", err)
			} else if latestVersion != "" {
				if cfg.AutoUpdate {
					console.Info("New version available (%s). Auto-updating...", latestVersion)
					if err := update.ApplyUpdate(console, version); err != nil {
						console.Error("Auto-update failed: %v", err)
					}
					// Exit after attempting update, successful or not. User should re-run.
					os.Exit(0)
				} else {
					console.Warn("A new version of tikwm is available: %s. Run 'tikwm update' to upgrade.", console.Bold.Sprint(latestVersion))
				}
			}
		}

		return nil
	},
	PersistentPostRunE: func(cmd *cobra.Command, args []string) error {
		// Stop the global rate limiter.
		tikwm.StopRateLimiter()
		// Close the database connection.
		if database != nil {
			return database.Close()
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		// Run the default download command.
		return runDownload(cmd, args)
	},
	SilenceUsage:  true,
	SilenceErrors: true,
}

// init initializes the command line interface.
func init() {
	// Initialize the console.
	console = cli.New(false)

	// Initialize cobra.
	cobra.OnInitialize(func() {
		// Check if quiet mode is enabled.
		if val, err := rootCmd.Flags().GetBool("quiet"); err == nil && val {
			flagQuiet = true
			console = cli.New(true)
		}

		var err error
		// Get the config path from the flags.
		if val, err := rootCmd.Flags().GetString("config"); err == nil {
			flagConfigPath = val
		}

		// Load the config file.
		cfg, err = cliconfig.Load(flagConfigPath)
		if err != nil {
			console.Error("Error loading config: %v", err)
			os.Exit(1)
		}

		// Apply command line flag overrides to the config.
		applyFlagOverrides(rootCmd, cfg)
	})

	rootCmd.Version = version
	rootCmd.SetVersionTemplate(`{{printf "%s\n" .Version}}`)

	// Define persistent flags that are available to all subcommands.
	rootCmd.PersistentFlags().StringVarP(&flagConfigPath, "config", "c", "", "Path to config file")
	rootCmd.PersistentFlags().BoolVarP(&flagQuiet, "quiet", "q", false, "Quiet mode, no console output except for errors")
	rootCmd.PersistentFlags().Bool("debug", false, "Log debug info to stderr and log file")
	rootCmd.PersistentFlags().Bool("clean-logs", false, "Redact sensitive info (usernames, IDs, paths) from log files")

	rootCmd.PersistentFlags().StringP("dir", "d", "", "Directory to save files (overrides config)")
	rootCmd.PersistentFlags().String("targets", "", "Path to a file with a list of targets (overrides config)")
	rootCmd.PersistentFlags().String("since", "", `Don't download videos earlier than this date (YYYY-MM-DD HH:MM:SS)`)
	rootCmd.PersistentFlags().StringP("quality", "", "", `Video quality to download ("hd", "sd", "all"). Overrides config.`)
	rootCmd.PersistentFlags().IntP("workers", "w", 0, "Number of concurrent workers (overrides config, default: num CPUs)")
	rootCmd.PersistentFlags().BoolP("force", "f", false, "Force download, ignore existing database entries")
	rootCmd.PersistentFlags().Bool("retry-on-429", false, "Retry with backoff on rate limit instead of falling back to SD")
	rootCmd.PersistentFlags().Bool("download-covers", false, `Enable downloading of post covers (see --cover-type).`)
	rootCmd.PersistentFlags().String("cover-type", "", `Cover type to download ("cover", "origin", "dynamic"). Overrides config.`)
	rootCmd.PersistentFlags().Bool("download-avatars", false, "Enable downloading of user avatars. Overrides config.")
	rootCmd.PersistentFlags().Bool("save-post-title", false, "Save post title to a .txt file. Overrides config.")

	// Network flags
	rootCmd.PersistentFlags().String("bind", "", "Outbound IP address or interface to bind to (overrides config)")

	// Caching flags
	rootCmd.PersistentFlags().Bool("feed-cache", false, "Enable or disable caching of user feeds. Overrides config.")
	rootCmd.PersistentFlags().String("feed-cache-ttl", "", `Time-to-live for feed cache, e.g., "1h", "30m". Overrides config.`)

	// Daemon flags
	rootCmd.PersistentFlags().Bool("daemon", false, "Enable daemon mode for continuous, low-frequency polling. Overrides config.")
	rootCmd.PersistentFlags().String("daemon-poll-interval", "", `Polling interval for daemon mode, e.g., "60s". Overrides config.`)

	// Add subcommands.
	rootCmd.AddCommand(downloadCmd)
	rootCmd.AddCommand(infoCmd)
	rootCmd.AddCommand(editCmd)
	rootCmd.AddCommand(coversCmd)
	rootCmd.AddCommand(fixCmd)
	rootCmd.AddCommand(debugCmd)
	rootCmd.AddCommand(updateCmd)
}

// Execute executes the root command.
func Execute() error {
	return rootCmd.Execute()
}
