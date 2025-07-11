package tikwm

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"runtime"
	"time"

	"github.com/cavaliergopher/grab/v3"
	"github.com/perpetuallyhorni/tikwm/internal/fs"
)

// ErrDiskSpace is returned when there is not enough disk space to perform a download.
var ErrDiskSpace = errors.New("insufficient disk space")

const (
	// MinRequiredDiskSpace is the minimum disk space required (in bytes) for downloads
	// where the file size is not known in advance (e.g., covers, avatars).
	MinRequiredDiskSpace = 10 * 1024 * 1024 // 10MB
)

// DownloadOpt holds the options for downloading content.
type DownloadOpt struct {
	Directory      string                                              // The directory to save downloaded files to.
	DownloadWith   func(url string, filename string) error             // Function to download the file from a URL to a filename.
	ValidateWith   func(filename string) (bool, error)                 // Function to validate the downloaded file.
	FilenameFormat func(post *Post, i int, assetType AssetType) string // Function to format the filename of the downloaded file.
	Timeout        time.Duration                                       // Timeout for the download operation.
	TimeoutOnError time.Duration                                       // Timeout between retries on error.
	NoSync         bool                                                // Disable synchronization lock for concurrent downloads.
	Retries        int                                                 // Number of retries for download attempts.
	FfmpegPath     string                                              // Path to the ffmpeg executable for validation.
}

// FileSHA256 calculates the SHA256 hash of a file.
func FileSHA256(path string) (string, error) {
	f, err := os.Open(path) // #nosec G304 // Open the file at the given path.
	if err != nil {
		return "", fmt.Errorf("failed to open file: %w", err) // Return an error if the file cannot be opened.
	}
	defer func() {
		if err := f.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Error closing file: %v\n", err) // Close the file when the function exits, handling potential errors.
		}
	}()

	h := sha256.New() // Create a new SHA256 hasher.
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("failed to copy file to hasher: %w", err) // Copy the file contents to the hasher.
	}
	return hex.EncodeToString(h.Sum(nil)), nil // Return the hexadecimal representation of the SHA256 hash.
}

// DownloadAndHash downloads a file from a URL to a specific path and returns its SHA256 hash.
func DownloadAndHash(url, fullPath string) (string, error) {
	dir := path.Dir(fullPath)
	available, err := fs.Available(dir)
	if err != nil {
		// Do not retry on disk check errors, treat as fatal for this operation.
		return "", fmt.Errorf("could not check disk space for %s: %w", dir, err)
	}
	if available < MinRequiredDiskSpace {
		// Do not retry if disk space is insufficient.
		return "", fmt.Errorf("%w: %d bytes available in %s, requires at least %d bytes", ErrDiskSpace, available, dir, MinRequiredDiskSpace)
	}

	req, err := grab.NewRequest(fullPath, url) // Create a new download request.
	if err != nil {
		return "", err // Return an error if the request cannot be created.
	}
	if resp := DefaultDownloadClient.Do(req); resp.Err() != nil { // Execute the download request.
		return "", resp.Err() // Return an error if the download fails.
	}

	hash, err := FileSHA256(fullPath) // Calculate the SHA256 hash of the downloaded file.
	if err != nil {
		_ = os.Remove(fullPath)                                                       // Clean up failed download
		return "", fmt.Errorf("failed to hash downloaded file %s: %w", fullPath, err) // Return an error if hashing fails.
	}
	return hash, nil // Return the SHA256 hash and nil error.
}

// ValidateWithFfmpeg returns a validation function that uses ffmpeg to decode the entire file.
// This is a robust way to check for corruption or truncation.
func ValidateWithFfmpeg(ffmpegPath string) func(filename string) (bool, error) {
	ffmpeg := "ffmpeg" // Default to ffmpeg in PATH
	if ffmpegPath != "" {
		ffmpeg = ffmpegPath // Use the provided ffmpeg path if available.
	}
	// Determine the null output device based on the OS.
	nullDevice := "/dev/null" // Default null device for Linux/macOS
	if runtime.GOOS == "windows" {
		nullDevice = "NUL" // Null device for Windows.
	}

	return func(filename string) (bool, error) {
		// Use -v error to suppress normal output. -i specifies the input.
		// -f null forces ffmpeg to process/decode the file but discard the output.
		// If the file is corrupt or truncated, ffmpeg will exit with a non-zero status.
		cmd := exec.Command(ffmpeg, "-v", "error", "-i", filename, "-f", "null", nullDevice) // Create the ffmpeg command.
		output, err := cmd.CombinedOutput()                                                  // Capture stderr for error messages
		if err != nil {
			return false, fmt.Errorf("ffmpeg validation failed for %s: %w\nOutput:\n%s", filename, err, string(output)) // Return false and an error if ffmpeg fails.
		}
		// If ffmpeg exits successfully, the entire file was processed without error.
		return true, nil // Return true if ffmpeg successfully processed the file.
	}
}

// Defaults sets default values for the DownloadOpt.
func (opt *DownloadOpt) Defaults() *DownloadOpt {
	ret := opt
	if ret == nil {
		ret = &DownloadOpt{}
	}
	if ret.DownloadWith == nil {
		ret.DownloadWith = func(url string, filename string) error {
			req, err := grab.NewRequest(filename, url)
			if err != nil {
				return err
			}
			if resp := DefaultDownloadClient.Do(req); resp.Err() != nil {
				return resp.Err()
			}
			return nil
		}
	}
	// Default validation is now ffmpeg if the path is provided.
	if ret.ValidateWith == nil {
		if ret.FfmpegPath != "" {
			ret.ValidateWith = ValidateWithFfmpeg(ret.FfmpegPath)
		} else {
			// If no ffmpeg path, default to a no-op validator.
			ret.ValidateWith = func(filename string) (bool, error) { return true, nil }
		}
	}
	if ret.FilenameFormat == nil {
		ret.FilenameFormat = formatFilename
	}
	if ret.Timeout == 0 {
		ret.Timeout = DefaultTimeout
	}
	if ret.TimeoutOnError == 0 {
		ret.TimeoutOnError = DefaultTimeoutOnError
	}
	if ret.Retries <= 0 {
		ret.Retries = 3
	}
	return ret
}

// DefaultDownloadClient is the default HTTP client for downloading files.
var (
	DefaultDownloadClient = &grab.Client{
		HTTPClient: &http.Client{
			Transport: &http.Transport{
				Proxy: http.ProxyFromEnvironment,
			},
			Timeout: time.Minute * 5,
		},
		UserAgent: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/16.6 Safari/605.1.1",
	}
	DefaultTimeout        = time.Millisecond * 100 // Default timeout for requests.
	DefaultTimeoutOnError = time.Second * 10       // Default timeout between retries on error.
)

// IsAlbum returns true if the post is an album (has images).
func (post Post) IsAlbum() bool {
	return len(post.Images) != 0
}

// IsVideo returns true if the post is a video (not an album).
func (post Post) IsVideo() bool {
	return !post.IsAlbum()
}

// formatFilename formats the filename for downloaded files.
func formatFilename(post *Post, i int, assetType AssetType) string {
	base := fmt.Sprintf("%s_%s_%s", post.Author.UniqueId, time.Unix(post.CreateTime, 0).Format(time.DateOnly), post.ID())
	if post.IsVideo() {
		return fmt.Sprintf("%s_%s.mp4", base, assetType)
	}
	return fmt.Sprintf("%s_%d.jpg", base, i+1)
}
