package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/inconshreveable/go-update"
	"github.com/perpetuallyhorni/tikwm/tools/tikwm/internal/cli"
)

const (
	repoOwner        = "perpetuallyhorni"
	repoName         = "tikwm"
	latestReleaseURL = "https://api.github.com/repos/perpetuallyhorni/tikwm/releases/latest"
)

// githubRelease represents the structure of a GitHub release API response.
type githubRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name        string `json:"name"`
		DownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

// version represents a parsed version string.
type version struct {
	Major int
	Minor int
}

// parseVersion parses a string like "v1.01" into a version struct.
func parseVersion(vStr string) (version, error) {
	vStr = strings.TrimPrefix(vStr, "v")
	parts := strings.Split(vStr, ".")
	if len(parts) != 2 {
		return version{}, fmt.Errorf("invalid version format: %s", vStr)
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return version{}, fmt.Errorf("invalid major version: %w", err)
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return version{}, fmt.Errorf("invalid minor version: %w", err)
	}
	return version{Major: major, Minor: minor}, nil
}

// lessThan compares two versions.
func (v version) lessThan(other version) bool {
	if v.Major < other.Major {
		return true
	}
	if v.Major == other.Major && v.Minor < other.Minor {
		return true
	}
	return false
}

// getLatestRelease fetches the latest release information from GitHub.
func getLatestRelease() (*githubRelease, error) {
	req, err := http.NewRequest(http.MethodGet, latestReleaseURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch latest release info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bad status from GitHub API: %s", resp.Status)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("failed to decode release info: %w", err)
	}
	return &release, nil
}

// CheckForUpdate checks for a new version on GitHub.
// It returns the latest version tag if an update is available, otherwise an empty string.
func CheckForUpdate(currentVersion string) (string, error) {
	if currentVersion == "dev" {
		return "", nil
	}
	release, err := getLatestRelease()
	if err != nil {
		return "", err
	}

	current, err := parseVersion(currentVersion)
	if err != nil {
		return "", fmt.Errorf("failed to parse current version: %w", err)
	}
	latest, err := parseVersion(release.TagName)
	if err != nil {
		return "", fmt.Errorf("failed to parse latest version tag: %w", err)
	}

	if current.lessThan(latest) {
		return release.TagName, nil
	}

	return "", nil
}

// getArchName maps Go's runtime.GOARCH to the goreleaser archive name format.
func getArchName() string {
	arch := runtime.GOARCH
	if arch == "amd64" {
		return "x86_64"
	}
	return arch
}

// getAssetName constructs the expected asset filename.
func getAssetName() string {
	ext := "tar.gz"
	if runtime.GOOS == "windows" {
		ext = "zip"
	}
	return fmt.Sprintf("%s_%s_%s.%s", repoName, runtime.GOOS, getArchName(), ext)
}

// extractFileFromArchive extracts the binary from the downloaded archive.
func extractFileFromArchive(body io.Reader, filename string) (io.Reader, error) {
	var binData []byte
	var binFound bool

	archiveBody, err := io.ReadAll(body)
	if err != nil {
		return nil, fmt.Errorf("failed to read archive body: %w", err)
	}

	binaryName := "tikwm"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}

	if strings.HasSuffix(filename, ".zip") {
		r, err := zip.NewReader(bytes.NewReader(archiveBody), int64(len(archiveBody)))
		if err != nil {
			return nil, fmt.Errorf("failed to create zip reader: %w", err)
		}
		for _, f := range r.File {
			if !f.FileInfo().IsDir() && filepath.Base(f.Name) == binaryName {
				rc, err := f.Open()
				if err != nil {
					return nil, fmt.Errorf("failed to open file in zip: %w", err)
				}
				binData, err = io.ReadAll(rc)
				defer func() {
					if err := rc.Close(); err != nil {
						fmt.Fprintf(os.Stderr, "Error closing file: %v\n", err)
					}
				}()
				if err != nil {
					return nil, fmt.Errorf("failed to read executable from zip: %w", err)
				}
				binFound = true
				break
			}
		}
	} else { // .tar.gz
		gzr, err := gzip.NewReader(bytes.NewReader(archiveBody))
		if err != nil {
			return nil, fmt.Errorf("failed to create gzip reader: %w", err)
		}
		defer gzr.Close()
		tr := tar.NewReader(gzr)
		for {
			header, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, fmt.Errorf("tar reading error: %w", err)
			}
			if header.Typeflag == tar.TypeReg && filepath.Base(header.Name) == binaryName {
				binData, err = io.ReadAll(tr)
				if err != nil {
					return nil, fmt.Errorf("failed to read executable from tarball: %w", err)
				}
				binFound = true
				break
			}
		}
	}

	if !binFound {
		return nil, fmt.Errorf("executable '%s' not found in archive", binaryName)
	}
	return bytes.NewReader(binData), nil
}

// ApplyUpdate performs the self-update to the latest version.
func ApplyUpdate(console *cli.Console, currentVersion string) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("could not locate executable path: %w", err)
	}
	if strings.Contains(exe, "go-build") {
		console.Error("Update command cannot be used with `go run`.")
		console.Info("Please build or install the binary first, then run the update on the compiled executable.")
		return nil
	}

	if currentVersion == "dev" {
		console.Warn("Cannot update 'dev' version.")
		return nil
	}
	console.Info("Checking for latest version...")
	release, err := getLatestRelease()
	if err != nil {
		return err
	}

	current, err := parseVersion(currentVersion)
	if err != nil {
		return fmt.Errorf("failed to parse current version '%s': %w", currentVersion, err)
	}
	latest, err := parseVersion(release.TagName)
	if err != nil {
		return fmt.Errorf("failed to parse latest version tag '%s': %w", release.TagName, err)
	}

	if !current.lessThan(latest) {
		console.Success("You are already using the latest version of tikwm (%s).", currentVersion)
		return nil
	}

	console.Info("Updating from %s to %s...", currentVersion, release.TagName)

	assetName := getAssetName()
	var assetURL string
	for _, asset := range release.Assets {
		if asset.Name == assetName {
			assetURL = asset.DownloadURL
			break
		}
	}

	if assetURL == "" {
		return fmt.Errorf("could not find update asset '%s' for this platform", assetName)
	}

	console.Info("Downloading: %s", assetName)
	resp, err := http.Get(assetURL) // #nosec G107
	if err != nil {
		return fmt.Errorf("failed to download asset: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status downloading asset: %s", resp.Status)
	}

	bin, err := extractFileFromArchive(resp.Body, assetName)
	if err != nil {
		return fmt.Errorf("failed to extract binary: %w", err)
	}

	console.Info("Applying update...")
	err = update.Apply(bin, update.Options{})
	if err != nil {
		return fmt.Errorf("update apply failed: %w", err)
	}

	console.Success("Successfully updated to version %s", release.TagName)
	console.Info("If you executed a command other than 'update', please run your command again.")
	return nil
}
