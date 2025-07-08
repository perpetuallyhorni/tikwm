package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/adrg/xdg"
	"github.com/perpetuallyhorni/tikwm/pkg/config"
	"github.com/spf13/cobra"
)

// editCmd is the parent command for editing configuration files.
var editCmd = &cobra.Command{
	Use:   "edit",
	Short: "Edit configuration or targets file in your default editor.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

// editConfigCmd is the command for editing the main configuration file.
var editConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "Edit the configuration file.",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get the configuration file path from the flag or default location.
		configFilePath, _ := cmd.Flags().GetString("config")
		if configFilePath == "" {
			var findErr error
			configFilePath, findErr = xdg.ConfigFile(filepath.Join(config.AppName, "config.yaml"))
			if findErr != nil {
				return fmt.Errorf("could not determine default config file path: %w", findErr)
			}
		}
		// Ensure the directory exists before trying to open the file.
		if err := os.MkdirAll(filepath.Dir(configFilePath), 0750); err != nil {
			return fmt.Errorf("could not create config directory: %w", err)
		}

		// Determine the editor to use.
		editor, err := determineEditor(cmd)
		if err != nil {
			return err
		}

		console.Info("Opening config file with '%s': %s", editor, configFilePath)
		return openInEditor(editor, configFilePath)
	},
}

// editTargetsCmd is the command for editing the targets file.
var editTargetsCmd = &cobra.Command{
	Use:   "targets",
	Short: "Edit the targets file.",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Check if targets file is defined in config.
		if cfg.TargetsFile == "" {
			return fmt.Errorf("targets file path is not defined in config")
		}
		// Ensure the directory exists before trying to open the file.
		if err := os.MkdirAll(filepath.Dir(cfg.TargetsFile), 0750); err != nil {
			return fmt.Errorf("could not create targets directory: %w", err)
		}

		// Determine the editor to use.
		editor, err := determineEditor(cmd)
		if err != nil {
			return err
		}

		console.Info("Opening targets file with '%s': %s", editor, cfg.TargetsFile)
		return openInEditor(editor, cfg.TargetsFile)
	},
}

// determineEditor selects the editor to use based on flag, config, env var, and fallbacks.
func determineEditor(cmd *cobra.Command) (string, error) {
	// 1. Command-line flag (highest priority)
	if editor, _ := cmd.Flags().GetString("editor"); editor != "" {
		return editor, nil
	}

	// 2. Configuration file
	if cfg.Editor != "" {
		return cfg.Editor, nil
	}

	// 3. Environment variable
	if editor := os.Getenv("EDITOR"); editor != "" {
		return editor, nil
	}

	// 4. Hardcoded fallbacks
	switch runtime.GOOS {
	case "windows":
		return "notepad", nil
	default:
		for _, editor := range []string{"nano", "vi", "vim"} {
			if path, err := exec.LookPath(editor); err == nil {
				return path, nil
			}
		}
	}

	return "", fmt.Errorf("no suitable editor found. please set the --editor flag, 'editor' in your config, or the $EDITOR environment variable")
}

// openInEditor opens the specified file in the given editor.
func openInEditor(editor, filePath string) error {
	// #nosec G204 -- The editor is determined from trusted sources (config, env, flags) or safe fallbacks.
	cmd := exec.Command(editor, filePath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// init initializes the edit command and its subcommands.
func init() {
	editCmd.PersistentFlags().String("editor", "", "Editor to use for opening files (e.g., 'code', 'vim', 'notepad'). Overrides config and $EDITOR.")
	editCmd.AddCommand(editConfigCmd)
	editCmd.AddCommand(editTargetsCmd)
}
