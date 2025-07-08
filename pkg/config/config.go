package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/adrg/xdg"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

const AppName = "tikwm" // Define the application name.

// Config struct holds the application configuration.
type Config struct {
	DownloadPath    string `koanf:"download_path"`    // Path to download videos and images.
	TargetsFile     string `koanf:"targets_file"`     // Path to the targets file.
	DatabasePath    string `koanf:"database_path"`    // Path to the SQLite database.
	Quality         string `koanf:"quality"`          // Quality of the downloaded videos ("source", "hd", "sd", "all").
	Since           string `koanf:"since"`            // Date to download content since (YYYY-MM-DD HH:MM:SS).
	RetryOn429      bool   `koanf:"retry_on_429"`     // Retry download on 429 error.
	DownloadCovers  bool   `koanf:"download_covers"`  // Download video cover images.
	CoverType       string `koanf:"cover_type"`       // Type of cover to download ("cover", "origin", "dynamic").
	DownloadAvatars bool   `koanf:"download_avatars"` // Download user profile avatars.
	SavePostTitle   bool   `koanf:"save_post_title"`  // Save the post title to a .txt file.
	FfmpegPath      string `koanf:"ffmpeg_path"`      // Path to the ffmpeg executable.
	Editor          string `koanf:"editor"`           // Editor to use for editing.
}

// Default returns the default configuration.
func Default() (*Config, error) {
	dbPath, err := xdg.DataFile(filepath.Join(AppName, "history.db")) // Get the default database path.
	if err != nil {
		return nil, fmt.Errorf("failed to get default db path: %w", err) // Return error if failed.
	}
	targetsPath, err := xdg.DataFile(filepath.Join(AppName, "targets.txt")) // Get the default targets file path.
	if err != nil {
		return nil, fmt.Errorf("failed to get default targets path: %w", err) // Return error if failed.
	}
	return &Config{ // Return the default configuration.
		DownloadPath:    "./downloads",         // Default download path.
		TargetsFile:     targetsPath,           // Default targets file path.
		DatabasePath:    dbPath,                // Default database path.
		Quality:         "source",              // Default quality.
		Since:           "1970-01-01 00:00:00", // Default since date.
		RetryOn429:      false,                 // Default retry on 429.
		DownloadCovers:  false,                 // Default download covers.
		CoverType:       "cover",               // Default cover type.
		DownloadAvatars: false,                 // Default download avatars.
		SavePostTitle:   false,                 // Default save post title.
		FfmpegPath:      "ffmpeg",              // Default ffmpeg path.
		Editor:          "",                    // Default editor.
	}, nil
}

// Load loads the configuration from the given path.
func Load(path string) (*Config, error) {
	k := koanf.New(".")      // Create a new Koanf instance.
	defCfg, err := Default() // Get the default configuration.
	if err != nil {
		return nil, err // Return error if failed.
	}
	cfgPath := path    // Set the config path.
	if cfgPath == "" { // If the config path is empty.
		cfgPath, err = xdg.ConfigFile(filepath.Join(AppName, "config.yaml")) // Get the default config path.
		if err != nil {
			return nil, fmt.Errorf("failed to get default config path: %w", err) // Return error if failed.
		}
	}
	if _, err := os.Stat(cfgPath); errors.Is(err, os.ErrNotExist) { // If the config file does not exist.
		if err := createDefaultConfig(cfgPath, defCfg); err != nil { // Create the default config file.
			return nil, fmt.Errorf("failed to create default config: %w", err) // Return error if failed.
		}
	}
	if err := k.Load(file.Provider(cfgPath), yaml.Parser()); err != nil { // Load the config file.
		return nil, fmt.Errorf("failed to load config file: %w", err) // Return error if failed.
	}
	cfg := defCfg                                // Set the config to the default config.
	if err := k.Unmarshal("", cfg); err != nil { // Unmarshal the config.
		return nil, fmt.Errorf("failed to unmarshal config: %w", err) // Return error if failed.
	}

	// If the user's config specifies an empty string for targets_file,
	// fall back to the new default path to avoid errors.
	if cfg.TargetsFile == "" {
		cfg.TargetsFile = defCfg.TargetsFile
	}

	if _, err := os.Stat(cfg.TargetsFile); errors.Is(err, os.ErrNotExist) { // If the targets file does not exist.
		if err := createDefaultTargetsFile(cfg.TargetsFile); err != nil { // Create the default targets file.
			// Not a fatal error, just warn the user.
			fmt.Fprintf(os.Stderr, "Warning: failed to create default targets file: %v\n", err) // Print a warning message.
		}
	}
	return cfg, nil // Return the config.
}

// createDefaultConfig creates a default configuration file.
func createDefaultConfig(path string, cfg *Config) error {
	dir := filepath.Dir(path)                      // Get the directory of the config file.
	if err := os.MkdirAll(dir, 0750); err != nil { // Create the directory if it does not exist.
		return fmt.Errorf("failed to create config directory: %w", err) // Return error if failed.
	}
	content := fmt.Sprintf(`# tikwm CLI configuration file.
# Path where videos and images will be downloaded.
download_path: "%s"
# Path to a file containing a list of targets (usernames or URLs), one per line.
# This file is used if no targets are provided on the command line.
targets_file: "%s"
# Path to the SQLite database to track downloaded posts.
database_path: "%s"
# Quality to download videos in. Options: "source", "hd", "sd", "all".
quality: "%s"
# Default date to download content since (YYYY-MM-DD HH:MM:SS).
since: "%s"
# Set to true to download video cover images along with the video.
download_covers: %t
# Type of cover to download. Options:
# "cover" or "medium": The standard, medium-quality cover.
# "origin" or "small": A slightly smaller, lower-qualtiy cover.
# "dynamic": An animated dynamic cover.
cover_type: "%s"
# Set to true to download user profile avatars.
download_avatars: %t
# Set to true to save the post title to a .txt file.
save_post_title: %t
# When rate-limited (429) on an HD link, retry with backoff or fall back to SD?
# Set to true to retry with backoff, false to fall back to SD.
retry_on_429: %t
# Path to the ffmpeg executable. Used to validate downloaded videos.
ffmpeg_path: "%s"
# Editor to use for the 'edit' command. If empty, it will check $EDITOR, then common editors.
editor: "%s"
`, cfg.DownloadPath, cfg.TargetsFile, cfg.DatabasePath, cfg.Quality, cfg.Since, cfg.DownloadCovers, cfg.CoverType, cfg.DownloadAvatars, cfg.SavePostTitle, cfg.RetryOn429, cfg.FfmpegPath, cfg.Editor) // Format the config file content.
	content = strings.ReplaceAll(content, "\\", "/")                  // Replace backslashes with forward slashes.
	if err := os.WriteFile(path, []byte(content), 0600); err != nil { // Write the config file.
		return fmt.Errorf("failed to write default config file: %w", err) // Return error if failed.
	}
	return nil // Return nil if successful.
}

// createDefaultTargetsFile creates a default targets file.
func createDefaultTargetsFile(path string) error {
	dir := filepath.Dir(path)                      // Get the directory of the targets file.
	if err := os.MkdirAll(dir, 0750); err != nil { // Create the directory if it does not exist.
		return fmt.Errorf("failed to create targets directory: %w", err) // Return error if failed.
	}
	content := `# Add TikTok usernames or video URLs here, one per line.
# Lines starting with # are ignored.
#
# Example:
# losertron
# https://www.tiktok.com/@creator/video/12345
` // Default targets file content.
	return os.WriteFile(path, []byte(content), 0600) // Write the targets file.
}
