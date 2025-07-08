package tikwm

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/cavaliergopher/grab/v3"
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

// getURLForAsset retrieves the download URL for a given asset type.
func getURLForAsset(post *Post, assetType AssetType) (string, error) {
	switch assetType {
	case AssetHD:
		return post.Hdplay, nil // Return the HD play URL.
	case AssetSD:
		return post.Play, nil // Return the SD play URL.
	case AssetSource:
		// This might get called multiple times in a retry loop, which is intended.
		sourceInfo, err := GetSourceEncode(post.ID()) // Get the source encode information.
		if err != nil {
			return "", fmt.Errorf("failed to get source encode URL: %w", err) // Return an error if the source encode URL cannot be retrieved.
		}
		if sourceInfo == nil || sourceInfo.PlayURL == "" {
			return "", fmt.Errorf("GetSourceEncode returned empty info/URL for post %s", post.ID()) // Return an error if the source encode URL is empty.
		}
		return sourceInfo.PlayURL, nil // Return the source encode play URL.
	default: // For album photos, assetType IS the URL
		if strings.HasPrefix(string(assetType), "http") {
			return string(assetType), nil // Return the URL as a string.
		}
		return "", fmt.Errorf("invalid asset type for URL retrieval: %s", assetType) // Return an error if the asset type is invalid.
	}
}

// downloadRetrying attempts to download a file with retries and post refresh on failures.
func (opt *DownloadOpt) downloadRetrying(post *Post, assetType AssetType, filename string, try int, lastErr error) error {
	// Base case: If we've exceeded the number of retries, fail.
	// If Retries is 3, we allow try 0, 1, 2, 3. The next attempt (try=4) fails.
	if try > opt.Retries {
		finalErr := lastErr
		if finalErr == nil {
			finalErr = fmt.Errorf("all retries failed")
		}
		return fmt.Errorf("failed after %d retries for post %s: %w", opt.Retries, post.ID(), finalErr) // Return an error after exceeding the maximum number of retries.
	}

	// If this is a retry attempt (try > 0), it means the previous attempt failed.
	// We MUST sleep and then potentially refresh the post object to get new download links.
	if try > 0 {
		time.Sleep(opt.TimeoutOnError) // Wait before retrying.
		// For source, the URL is fetched fresh every time by getURLForAsset.
		// For HD/SD, we need to refresh the post object itself.
		// Album photos also don't need a refresh, as the URL is static.
		if assetType == AssetHD || assetType == AssetSD {
			refreshedPost, refreshErr := GetPost(post.ID(), true) // Refresh the post object to get new download links.
			if refreshErr != nil {
				// If refreshing fails, we retry again, passing the refresh error as the new lastErr.
				return opt.downloadRetrying(post, assetType, filename, try+1, refreshErr)
			}
			*post = *refreshedPost // Update the post object with the new data.
		}
	}

	// Attempt to get the URL from the (potentially refreshed) post object.
	url, err := getURLForAsset(post, assetType) // Get the download URL for the asset type.
	if err != nil {
		// This can happen if, e.g., GetSourceEncode fails. Retry.
		return opt.downloadRetrying(post, assetType, filename, try+1, err)
	}

	// If the URL is STILL missing, even after a potential refresh, it's a failure for this attempt. Retry.
	if url == "" {
		return opt.downloadRetrying(post, assetType, filename, try+1, fmt.Errorf("URL for asset type %s is missing", assetType)) // Retry if the URL is still missing.
	}

	// Attempt to download the file.
	if err := opt.DownloadWith(url, filename); err != nil {
		// If download fails, retry.
		return opt.downloadRetrying(post, assetType, filename, try+1, err) // Retry if the download fails.
	}

	// For videos, attempt to validate the downloaded file.
	if post.IsVideo() {
		if valid, err := opt.ValidateWith(filename); err != nil {
			// If validation itself errors, retry.
			return opt.downloadRetrying(post, assetType, filename, try+1, err) // Retry if validation errors.
		} else if !valid {
			// If validation returns false (e.g., corrupt file), retry.
			return opt.downloadRetrying(post, assetType, filename, try+1, fmt.Errorf("validation failed for %s", filename)) // Retry if validation fails.
		}
	}

	// If all steps succeeded, we're done.
	return nil // Return nil if the download was successful.
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
	downloadSync          = &sync.Mutex{}          // Mutex for synchronizing downloads.
)

// IsAlbum returns true if the post is an album (has images).
func (post Post) IsAlbum() bool {
	return len(post.Images) != 0
}

// IsVideo returns true if the post is a video (not an album).
func (post Post) IsVideo() bool {
	return !post.IsAlbum()
}

// DownloadVideo downloads a specific quality of a video post.
func (post Post) DownloadVideo(assetType AssetType, opts ...DownloadOpt) (file string, sha256 string, err error) {
	switch assetType {
	case AssetHD, AssetSD, AssetSource:
		// Valid types
	default:
		return "", "", fmt.Errorf("unsupported asset type for video download: %s", assetType)
	}

	opt := &DownloadOpt{}
	if len(opts) != 0 {
		opt = &opts[0]
	}
	opt = opt.Defaults()
	if !opt.NoSync {
		downloadSync.Lock()
		defer downloadSync.Unlock()
	}

	creatorDir := path.Join(opt.Directory, post.Author.UniqueId)
	// #nosec G301
	if err := os.MkdirAll(creatorDir, 0755); err != nil {
		return "", "", fmt.Errorf("failed to create creator directory %s: %w", creatorDir, err)
	}

	filename := path.Join(creatorDir, opt.FilenameFormat(&post, 0, assetType))
	if err := opt.downloadRetrying(&post, assetType, filename, 0, nil); err != nil {
		return "", "", err
	}
	hash, err := FileSHA256(filename)
	if err != nil {
		_ = os.Remove(filename)
		return "", "", fmt.Errorf("failed to hash %s: %w", filename, err)
	}
	return filename, hash, nil
}

// DownloadAlbumPhoto downloads a single photo from an album post at a specific index.
func (post Post) DownloadAlbumPhoto(index int, opts ...DownloadOpt) (file string, sha256 string, err error) {
	if !post.IsAlbum() {
		return "", "", fmt.Errorf("post %s is not an album", post.ID())
	}
	if index < 0 || index >= len(post.Images) {
		return "", "", fmt.Errorf("index %d is out of bounds for album with %d images", index, len(post.Images))
	}

	opt := &DownloadOpt{}
	if len(opts) != 0 {
		opt = &opts[0]
	}
	opt = opt.Defaults()
	if !opt.NoSync {
		downloadSync.Lock()
		defer downloadSync.Unlock()
	}

	creatorDir := path.Join(opt.Directory, post.Author.UniqueId)
	// #nosec G301
	if err := os.MkdirAll(creatorDir, 0755); err != nil {
		return "", "", fmt.Errorf("failed to create creator directory %s: %w", creatorDir, err)
	}

	url := post.Images[index]
	filename := path.Join(creatorDir, opt.FilenameFormat(&post, index, ""))

	// Create a copy of the post for the retry logic to avoid race conditions if used concurrently.
	imgPost := post
	// Pass the direct URL as a temporary "AssetType" for the retry logic.
	if err := opt.downloadRetrying(&imgPost, AssetType(url), filename, 0, nil); err != nil {
		return "", "", err
	}

	hash, err := FileSHA256(filename)
	if err != nil {
		_ = os.Remove(filename)
		return "", "", fmt.Errorf("failed to hash %s: %w", filename, err)
	}
	return filename, hash, nil
}

// formatFilename formats the filename for downloaded files.
func formatFilename(post *Post, i int, assetType AssetType) string {
	base := fmt.Sprintf("%s_%s_%s", post.Author.UniqueId, time.Unix(post.CreateTime, 0).Format(time.DateOnly), post.ID())
	if post.IsVideo() {
		return fmt.Sprintf("%s_%s.mp4", base, assetType)
	}
	return fmt.Sprintf("%s_%d.jpg", base, i+1)
}
