package config

import (
	"path/filepath"

	"github.com/adrg/xdg"
)

// Config struct holds the core, application-agnostic configuration.
type Config struct {
	DownloadPath    string `koanf:"download_path"`    // Path to download videos and images.
	Quality         string `koanf:"quality"`          // Quality of the downloaded videos ("source", "hd", "sd", "all").
	Since           string `koanf:"since"`            // Date to download content since (YYYY-MM-DD HH:MM:SS).
	RetryOn429      bool   `koanf:"retry_on_429"`     // Retry download on 429 error.
	DownloadCovers  bool   `koanf:"download_covers"`  // Download video cover images.
	CoverType       string `koanf:"cover_type"`       // Type of cover to download ("cover", "origin", "dynamic").
	DownloadAvatars bool   `koanf:"download_avatars"` // Download user profile avatars.
	SavePostTitle   bool   `koanf:"save_post_title"`  // Save the post title to a .txt file.
	FfmpegPath      string `koanf:"ffmpeg_path"`      // Path to the ffmpeg executable.
}

// Default returns the default core configuration.
func Default() *Config {
	var defaultPath string
	downloadDir := xdg.UserDirs.Download
	if downloadDir != "" {
		defaultPath = filepath.Join(downloadDir, "tikwm")
	} else {
		// Fallback for systems without a configured XDG downloads directory.
		defaultPath = filepath.Join("downloads", "tikwm")
	}

	return &Config{
		DownloadPath:    defaultPath,
		Quality:         "source",
		Since:           "1970-01-01 00:00:00",
		RetryOn429:      false,
		DownloadCovers:  false,
		CoverType:       "cover",
		DownloadAvatars: false,
		SavePostTitle:   false,
		FfmpegPath:      "ffmpeg",
	}
}
