# tikwm

A downloader for TikTok, powered by tikwm.com

## Features

* Supports usernames (with or without `@`), profile links, and video URLs as targets.
* Download entire user profiles.
* Download Source, high-definition (HD), and standard-definition (SD) video qualities and track them separately.
* Download photo albums.
* Download post covers and user avatars (profile pictures).
* Save post titles to `.txt` files.
* Manage a list of targets (usernames or URLs) from a file.
* Download missing videos based on database records (`fix` command).
* Automatic update checks and notifications.
* Manual update command (`tikwm update`).
* Configurable auto-update behavior.
* Disk space check before downloading to prevent partial files.
* Log sanitization to avoid leaking sensitive data when reporting issues.
* Supports shell completion scripts (`completion` command).
* Quiet mode to suppress console output.
* Debug mode to log debug info to stderr and log file.
* SD Fallback/Exponential backoff: When downloading HD videos, if the user is running two or more instances of the program, it is possible that tikwm will return a 429 status code (Rate limited) to one of the instances, tikwm's rate limit is 1 request per second, in this case the user can choose if allowing fallback to a SD download or waiting and retrying the request for the HD video.

## Important files and directories

tikwm uses the XDG Base Directory Specification to store its files. The default locations are:

* **Download directory**:
    * Unix: `~/Downloads/tikwm/<username>`
    * MacOS: `~/Downloads/tikwm/<username>`
    * Windows: `%USERPROFILE%\Downloads\tikwm\<username>`
* **Config file**:
    * Unix: `~/.config/tikwm/config.yaml`
    * macOS: `~/Library/Application Support/tikwm/config.yaml`
    * Windows: `%APPDATA%\tikwm\config.yaml`
* **Targets file**:
    * Unix: `~/.local/share/tikwm/targets.txt`
    * macOS: `~/Library/Application Support/tikwm/targets.txt`
    * Windows: `%APPDATA%\tikwm\targets.txt`
* **History database**:
    * Unix: `~/.local/share/tikwm/history.db`
    * macOS: `~/Library/Application Support/tikwm/history.db`
    * Windows: `%APPDATA%\tikwm\history.db`
* **Log file**:
    * Unix: `~/.local/state/tikwm/app.log`
    * macOS: `~/Library/Logs/tikwm/app.log`
    * Windows: `%LOCALAPPDATA%\tikwm\app.log`

## Installation

1. **Download prebuilt binary:**
    Download the latest release from the [releases page](https://github.com/perpetuallyhorni/tikwm/releases).

2. **Build from source:**
    Ensure you have Go installed.

    ```bash
    go install github.com/perpetuallyhorni/tikwm/tools/tikwm@latest
    ```

## Usage

```bash
tikwm [command|targets...] [flags]
```

### Commands

* `download [targets...]`: Downloads posts or entire user profiles. This is the default command (This means you can omit the command name, e.g. `tikwm some_user`).
* `update`: Updates tikwm to the latest version.
* `info [targets...]`: Prints information about a user profile.
* `edit <config|targets>`: Edits the configuration or targets file in your default text editor (if you don't have the `EDITOR` environment variable set, you can define one in your config, or pass one with the --editor flag, e.g. `edit targets --editor notepad.exe`).
* `covers [targets...]`: Downloads missing cover images for users.
* `fix [targets...]`: Downloads videos that are missing the qualities specified in your config.
* `completion`: Generates shell completion script for bash, zsh, fish, or powershell.
* `help`: Shows general help for the tool.

### Arguments

* **targets**: A list of TikTok usernames or video URLs. If no command is specified, `download` is assumed.

### Flags

* `-c, --config string`: Path to the config file.
* `-q, --quiet`: Quiet mode, no console output except for errors.
* `--debug`: Log debug info.
* `--clean-logs`: Redact sensitive info from log files.
* `-d, --dir string`: Directory to save files (overrides config).
* `--targets string`: Path to a file with a list of targets (overrides config).
* `--since string`: Don't download videos earlier than this date (YYYY-MM-DD HH:MM:SS).
* `--quality string`: Video quality to download ("hd", "sd", "all").
* `-f, --force`: Force download, ignore existing database entries.
* `--retry-on-429`: Retry with backoff on rate limit instead of falling back to SD.
* `--download-covers`: Enable downloading of post covers.
* `--cover-type string`: Cover type to download ("cover", "origin", "dynamic").
* `--download-avatars`: Enable downloading of user avatars.
* `--save-post-title`: Save post title to a .txt file.

### Configuration

The application uses a YAML configuration file. You can edit it with `tikwm edit config`.

* `download_path`: Path where videos and images will be downloaded.
* `targets_file`: Path to a file containing a list of targets.
* `database_path`: Path to the SQLite database.
* `quality`: Video quality to download: "source", "hd", "sd", or "all".
* `since`: Download content since this date (YYYY-MM-DD HH:MM:SS).
* `download_covers`: Download video cover images.
* `cover_type`: Type of cover to download ("cover", "origin", "dynamic").
* `download_avatars`: Download user profile avatars.
* `save_post_title`: Save the post title to a .txt file.
* `retry_on_429`: Retry with backoff on rate limit.
* `ffmpeg_path`: Path to the FFmpeg executable (for video validation).
* `editor`: Text editor to use for the 'edit' command.
* `check_for_updates`: Check for new versions on startup.
* `auto_update`: Automatically install new versions.

### Examples

* Download a video by URL:

    ```bash
    tikwm https://www.tiktok.com/@some_user/video/12345
    ```

* Download a user's videos:

    ```bash
    tikwm some_user
    ```

* Download a user's videos, specifying HD quality:

    ```bash
    tikwm some_user --quality hd
    ```

* Download videos, using targets from a file:

    ```bash
    tikwm
    ```

    (Assuming a targets file is configured)

* Edit the configuration file:

    ```bash
    tikwm edit config
    ```

* Download missing covers for a user:

    ```bash
    tikwm covers some_user
    ```

* Fix missing videos for a user:

    ```bash
    tikwm fix some_user
    ```

* For the complete list of commands, options and detailed usage, run:

    ```bash
    tikwm help
    ```

* For detailed information about a specific command, run:

    ```bash
    tikwm <command> --help
    ```

## Library Usage

`tikwm` can also be used as a library in your own Go projects:

```bash
go get -u github.com/perpetuallyhorni/tikwm
```

### Example

This example demonstrates how to create a client, initialize a database, and download a user's entire profile.

```go
package main

import (
 "log"
 "os"

 "github.com/perpetuallyhorni/tikwm/pkg/client"
 "github.com/perpetuallyhorni/tikwm/pkg/config"
 "github.com/perpetuallyhorni/tikwm/pkg/storage/sqlite"
)

func main() {
 // 1. Use the default configuration.
 cfg := config.Default()
 cfg.DownloadPath = "./my_downloads" // Customize as needed.

 // 2. Initialize the SQLite database.
 // The client needs a storage backend that implements the `storage.Storer` interface.
 db, err := sqlite.New("./my_database.db")
 if err != nil {
  log.Fatalf("failed to initialize database: %v", err)
 }
 defer db.Close()

 // 3. Create a new client.
 appClient, err := client.New(cfg, db)
 if err != nil {
  log.Fatalf("failed to create client: %v", err)
 }

 // 4. Download a user's profile.
 username := "losertron"
 logger := log.New(os.Stdout, "", log.LstdFlags)

 log.Printf("Starting download for user: %s", username)
 err = appClient.DownloadProfile(username, false, logger, nil)
 if err != nil {
  log.Fatalf("failed to download profile: %v", err)
 }
 log.Printf("Successfully downloaded profile for user: %s", username)
}
```

### Extensible Database

The `sqlite.New` function returns a concrete `*sqlite.DB` type, which exposes the raw `*sql.DB` connection via its `Conn` field. This allows you to extend the database with your own tables and queries while still leveraging the core functionality provided by `tikwm`.

## Acknowledgements

> **@mehanon**, author of the [original tikwm API module](https://github.com/mehanon/tikwm), which this project is based on.</br>
> **[tikwm.com](https://tikwm.com/)**, for providing such a great API, and for being based as fuck.

## Disclaimer

This project is not affiliated with TikTok, ByteDance, or any of their subsidiaries or affiliates, and is not endorsed by them.

This project is not affiliated with tikwm.com, and is not endorsed by them.
