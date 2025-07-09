package cliconfig

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
	"github.com/perpetuallyhorni/tikwm/pkg/config"
)

const AppName = "tikwm"

// Config extends the core config with CLI-specific options.
type Config struct {
	config.Config `koanf:",squash"`
	TargetsFile   string `koanf:"targets_file"`
	DatabasePath  string `koanf:"database_path"`
	Editor        string `koanf:"editor"`
}

// Default returns the default CLI configuration.
func Default() (*Config, error) {
	coreCfg := config.Default()
	dbPath, err := xdg.DataFile(filepath.Join(AppName, "history.db"))
	if err != nil {
		return nil, fmt.Errorf("failed to get default db path: %w", err)
	}
	targetsPath, err := xdg.DataFile(filepath.Join(AppName, "targets.txt"))
	if err != nil {
		return nil, fmt.Errorf("failed to get default targets path: %w", err)
	}

	return &Config{
		Config:       *coreCfg,
		DatabasePath: dbPath,
		TargetsFile:  targetsPath,
		Editor:       "", // Default editor is determined in the 'edit' command logic
	}, nil
}

// Load loads the configuration from the given path.
func Load(path string) (*Config, error) {
	k := koanf.New(".")
	defCfg, err := Default()
	if err != nil {
		return nil, err
	}
	cfgPath := path
	if cfgPath == "" {
		cfgPath, err = xdg.ConfigFile(filepath.Join(AppName, "config.yaml"))
		if err != nil {
			return nil, fmt.Errorf("failed to get default config path: %w", err)
		}
	}
	if _, err := os.Stat(cfgPath); errors.Is(err, os.ErrNotExist) {
		if err := createDefaultConfig(cfgPath, defCfg); err != nil {
			return nil, fmt.Errorf("failed to create default config: %w", err)
		}
	}
	if err := k.Load(file.Provider(cfgPath), yaml.Parser()); err != nil {
		return nil, fmt.Errorf("failed to load config file: %w", err)
	}
	cfg := defCfg
	if err := k.Unmarshal("", cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// If the user's config specifies an empty string for targets_file,
	// fall back to the new default path to avoid errors.
	if cfg.TargetsFile == "" {
		cfg.TargetsFile = defCfg.TargetsFile
	}

	if _, err := os.Stat(cfg.TargetsFile); errors.Is(err, os.ErrNotExist) {
		if err := createDefaultTargetsFile(cfg.TargetsFile); err != nil {
			// Not a fatal error, just warn the user.
			fmt.Fprintf(os.Stderr, "Warning: failed to create default targets file: %v\n", err)
		}
	}
	return cfg, nil
}

// createDefaultConfig creates a default configuration file.
func createDefaultConfig(path string, cfg *Config) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
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
`, cfg.DownloadPath, cfg.TargetsFile, cfg.DatabasePath, cfg.Quality, cfg.Since, cfg.DownloadCovers, cfg.CoverType, cfg.DownloadAvatars, cfg.SavePostTitle, cfg.RetryOn429, cfg.FfmpegPath, cfg.Editor)
	content = strings.ReplaceAll(content, "\\", "/")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		return fmt.Errorf("failed to write default config file: %w", err)
	}
	return nil
}

// createDefaultTargetsFile creates a default targets file.
func createDefaultTargetsFile(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return fmt.Errorf("failed to create targets directory: %w", err)
	}
	content := `# Add TikTok usernames or video URLs here, one per line.
# Lines starting with # are ignored.
#
# Example:
# losertron
# https://www.tiktok.com/@creator/video/12345
`
	return os.WriteFile(path, []byte(content), 0600)
}
