package client

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	tikwm "github.com/perpetuallyhorni/tikwm/internal"
	"github.com/perpetuallyhorni/tikwm/pkg/config"
	"github.com/perpetuallyhorni/tikwm/pkg/storage"
)

// Client is the main entry point for interacting with the tikwm library.
type Client struct {
	cfg *config.Config
	db  storage.Storer
}

// New creates a new Client.
func New(cfg *config.Config, db storage.Storer) (*Client, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}
	if db == nil {
		return nil, fmt.Errorf("database cannot be nil")
	}
	return &Client{cfg: cfg, db: db}, nil
}

// ProgressCallback defines the function signature for progress reporting.
type ProgressCallback func(current, total int, message string)

// noOpProgress is a default empty progress callback.
func noOpProgress(current, total int, message string) {}

// ExtractUsername sanitizes a user target, which could be a name or a URL.
func ExtractUsername(target string) string {
	target = strings.TrimSpace(target)
	if u, err := url.Parse(target); err == nil && (u.Scheme == "http" || u.Scheme == "https") && strings.Contains(u.Host, "tiktok.com") {
		cleanPath := strings.Trim(u.Path, "/")
		parts := strings.Split(cleanPath, "/")
		if len(parts) > 0 && strings.HasPrefix(parts[0], "@") {
			return strings.TrimPrefix(parts[0], "@")
		}
	}
	return strings.TrimPrefix(target, "@")
}

// getQualitiesToDownload determines the asset types to download based on the configuration.
func (c *Client) getQualitiesToDownload() ([]tikwm.AssetType, error) {
	switch strings.ToLower(c.cfg.Quality) {
	case "source":
		return []tikwm.AssetType{tikwm.AssetSource}, nil
	case "hd":
		return []tikwm.AssetType{tikwm.AssetHD}, nil
	case "sd":
		return []tikwm.AssetType{tikwm.AssetSD}, nil
	case "all":
		return []tikwm.AssetType{tikwm.AssetSource, tikwm.AssetHD, tikwm.AssetSD}, nil
	default:
		return nil, fmt.Errorf("invalid quality '%s' in config, must be 'source', 'hd', 'sd', or 'all'", c.cfg.Quality)
	}
}

// getAssetPath constructs the full file path for a given asset.
func (c *Client) getAssetPath(post *tikwm.Post, assetType tikwm.AssetType) string {
	dOpts := (&tikwm.DownloadOpt{}).Defaults()
	var filename string

	switch assetType {
	case tikwm.AssetCoverMedium, tikwm.AssetCoverOrigin, tikwm.AssetCoverDynamic:
		filename = fmt.Sprintf("%s_%s_%s_%s.jpg", post.Author.UniqueId, time.Unix(post.CreateTime, 0).Format(time.DateOnly), post.ID(), assetType)
	default:
		// For HD/SD/Source videos, the asset index 'i' is always 0.
		filename = dOpts.FilenameFormat(post, 0, assetType)
	}

	return path.Join(c.cfg.DownloadPath, post.Author.UniqueId, filename)
}

// getCoverAssetType selects the correct cover asset type based on config.
func (c *Client) getCoverAssetType(post *tikwm.Post) (tikwm.AssetType, string) {
	switch strings.ToLower(c.cfg.CoverType) {
	case "origin", "small":
		return tikwm.AssetCoverOrigin, post.OriginCover
	case "dynamic":
		return tikwm.AssetCoverDynamic, post.AiDynamicCover
	case "cover", "medium":
		fallthrough
	default:
		return tikwm.AssetCoverMedium, post.Cover
	}
}

// checkLocalAsset checks if a file exists on disk and returns its size.
func (c *Client) checkLocalAsset(post *tikwm.Post, assetType tikwm.AssetType, logger *log.Logger) (exists bool, size int64, err error) {
	fullPath := c.getAssetPath(post, assetType)
	if fullPath == "" {
		return false, 0, nil // No valid path could be generated
	}
	logger.Printf("Checking filesystem for: %s", fullPath)

	info, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, 0, nil
		}
		// A different error occurred (e.g., permissions).
		return false, 0, err
	}

	logger.Printf("File exists on disk: %s (Size: %d)", fullPath, info.Size())
	return true, info.Size(), nil
}

// adoptLocalAsset calculates the hash of an existing local file and adds it to the database.
func (c *Client) adoptLocalAsset(post *tikwm.Post, assetType tikwm.AssetType, logger *log.Logger) error {
	fullPath := c.getAssetPath(post, assetType)
	if fullPath == "" {
		return fmt.Errorf("could not generate path to adopt asset for post %s", post.ID())
	}
	logger.Printf("Adopting local asset: %s", fullPath)

	hash, err := tikwm.FileSHA256(fullPath)
	if err != nil {
		return fmt.Errorf("failed to calculate hash for adoption of %s: %w", fullPath, err)
	}
	if hash == "" {
		return fmt.Errorf("calculated an empty hash for adoption of %s", fullPath)
	}

	logger.Printf("Successfully hashed local asset %s (SHA256: %s)", fullPath, hash)

	assetID := post.ID()
	if post.IsAlbum() {
		return fmt.Errorf("adoptLocalAsset should not be called for albums")
	}
	return c.db.AddOrUpdateAsset(assetID, post.Author.UniqueId, post.CreateTime, assetType, hash)
}

// DownloadPost downloads a single post by its URL.
func (c *Client) DownloadPost(url string, force bool, logger *log.Logger) error {
	post, err := tikwm.GetPost(url, true)
	if err != nil {
		return err
	}

	if post.IsVideo() {
		qualities, err := c.getQualitiesToDownload()
		if err != nil {
			return err
		}
		for _, assetType := range qualities {
			if err := c.ensureVideoAsset(post, assetType, force, logger); err != nil {
				logger.Printf("Could not process video for post %s (quality: %s): %v", post.ID(), assetType, err)
			}
		}
	} else if post.IsAlbum() {
		if err := c.ensureAlbum(post, force, logger); err != nil {
			logger.Printf("Could not process album for post %s: %v", post.ID(), err)
		}
	}

	if c.cfg.DownloadCovers {
		if err := c.ensureCoverAsset(post, force, logger); err != nil {
			logger.Printf("Could not download cover for post %s: %v", post.ID(), err)
		}
	}
	if c.cfg.DownloadAvatars {
		c.ensureAvatar(post, force, logger, make(map[string]bool))
	}
	return nil
}

// DownloadProfile orchestrates the download of a user's entire profile with optimizations.
func (c *Client) DownloadProfile(username string, force bool, logger *log.Logger, progressCb ProgressCallback) error {
	if progressCb == nil {
		progressCb = noOpProgress
	}
	qualitiesNeeded, err := c.getQualitiesToDownload()
	if err != nil {
		return err
	}
	since, err := time.Parse(time.DateTime, c.cfg.Since)
	if err != nil {
		return fmt.Errorf("invalid since date format: %w", err)
	}

	processedAvatars := make(map[string]bool)

	postChan, expectedCount, err := tikwm.GetUserFeed(username, &tikwm.FeedOpt{
		While: tikwm.WhileAfter(since),
		OnError: func(err error) {
			logger.Printf("Error during feed fetch for '%s': %v", username, err)
		},
		OnFeedProgress: func(count int) {
			progressCb(count, 0, fmt.Sprintf("%d posts found", count))
		},
	})
	if err != nil {
		return err
	}
	if expectedCount == 0 {
		logger.Printf("No new posts found for user %s since %s.", username, since.Format(time.DateOnly))
		return nil
	}

	i := 0
	for postFromFeed := range postChan {
		i++
		postID := postFromFeed.ID()
		progressCb(i, expectedCount, fmt.Sprintf("Checking %s", postID))
		logger.Printf("--- Checking post %s (%d/%d) ---", postID, i, expectedCount)

		if postFromFeed.IsAlbum() {
			c.processAlbumInFeed(&postFromFeed, force, logger, progressCb, i, expectedCount)
		} else { // Is a video
			c.processVideoInFeed(&postFromFeed, qualitiesNeeded, force, logger)
		}

		// Process cover for all post types
		if c.cfg.DownloadCovers {
			c.processCoverInFeed(&postFromFeed, force, logger)
		}
		// Process avatar once per author per run
		if c.cfg.DownloadAvatars {
			c.ensureAvatar(&postFromFeed, force, logger, processedAvatars)
		}
	}
	progressCb(expectedCount, expectedCount, "Profile processing complete.")
	return nil
}

// ensureAvatar handles downloading a user's avatar if it's new.
func (c *Client) ensureAvatar(post *tikwm.Post, force bool, logger *log.Logger, processed map[string]bool) {
	authorID := post.Author.UniqueId
	if _, ok := processed[authorID]; ok {
		return // Already handled this author in this session
	}
	processed[authorID] = true

	if post.Author.Avatar == "" {
		logger.Printf("No avatar URL found for author %s", authorID)
		return
	}

	logger.Printf("Processing avatar for %s...", authorID)

	creatorDir := path.Join(c.cfg.DownloadPath, authorID)
	// #nosec G301
	if err := os.MkdirAll(creatorDir, 0755); err != nil {
		logger.Printf("Could not create directory for avatar for %s: %v", authorID, err)
		return
	}

	// Download to a temporary path to get the hash first
	tempPath := filepath.Join(creatorDir, fmt.Sprintf("avatar_temp_%d", time.Now().UnixNano()))
	hash, err := tikwm.DownloadAndHash(post.Author.Avatar, tempPath)
	if err != nil {
		logger.Printf("Failed to download avatar for %s: %v", authorID, err)
		_ = os.Remove(tempPath)
		return
	}

	exists, err := c.db.AvatarExists(authorID, hash)
	if err != nil {
		logger.Printf("Failed to check DB for avatar for %s: %v", authorID, err)
		_ = os.Remove(tempPath)
		return
	}

	if exists && !force {
		logger.Printf("Avatar for %s (hash: %s) already exists in database. Discarding.", authorID, hash)
		_ = os.Remove(tempPath)
		return
	}

	// If it doesn't exist or we're forcing, rename to final destination and add to DB
	timestamp := time.Now().UTC().Format("20060102T150405Z")
	finalPath := filepath.Join(creatorDir, fmt.Sprintf("%s_%s_avatar.jpg", authorID, timestamp))

	if err := os.Rename(tempPath, finalPath); err != nil {
		logger.Printf("Failed to move avatar to final destination for %s: %v", authorID, err)
		_ = os.Remove(tempPath)
		return
	}

	if err := c.db.AddAvatar(authorID, hash); err != nil {
		logger.Printf("Failed to add avatar to DB for %s: %v", authorID, err)
		return
	}
	logger.Printf("Successfully downloaded new avatar for %s to %s", authorID, finalPath)
}

// savePostTitle saves the post's title to a single, quality-agnostic text file.
func (c *Client) savePostTitle(post *tikwm.Post, logger *log.Logger) error {
	if !c.cfg.SavePostTitle || post.Title == "" {
		return nil
	}

	baseFilename := fmt.Sprintf("%s_%s_%s", post.Author.UniqueId, time.Unix(post.CreateTime, 0).Format(time.DateOnly), post.ID())
	txtPath := filepath.Join(c.cfg.DownloadPath, post.Author.UniqueId, baseFilename+".txt")

	// Check if the file already exists to avoid redundant writes.
	if _, err := os.Stat(txtPath); err == nil {
		return nil
	}

	logger.Printf("Saving title for post %s to %s", post.ID(), txtPath)
	// #nosec G306
	return os.WriteFile(txtPath, []byte(post.Title), 0644)
}

// processVideoInFeed handles video-specific processing within the feed.
func (c *Client) processVideoInFeed(postFromFeed *tikwm.Post, qualitiesNeeded []tikwm.AssetType, force bool, logger *log.Logger) {
	postID := postFromFeed.ID()
	validator := tikwm.ValidateWithFfmpeg(c.cfg.FfmpegPath)

	if force {
		logger.Printf("Force enabled for %s. Fetching full details to download all qualities.", postID)
		fullPost, err := tikwm.GetPost(postID, true)
		if err != nil {
			logger.Printf("Failed to get full post details for %s: %v", postID, err)
			return
		}
		for _, quality := range qualitiesNeeded {
			if err := c.ensureVideoAsset(fullPost, quality, true, logger); err != nil {
				logger.Printf("Error during forced download for %s (quality: %s): %v", postID, quality, err)
			}
		}
		return
	}

	for _, quality := range qualitiesNeeded {
		if exists, _ := c.db.AssetExists(postID, quality); exists {
			continue
		}

		exists, size, err := c.checkLocalAsset(postFromFeed, quality, logger)
		if err != nil {
			logger.Printf("Error checking local asset for %s (quality: %s): %v. Will attempt download.", postID, quality, err)
			if err := c.ensureVideoAsset(postFromFeed, quality, true, logger); err != nil {
				logger.Printf("Error downloading video for %s: %v", postID, err)
			}
			continue
		}

		if !exists {
			logger.Printf("Asset for %s (quality: %s) not found. Downloading.", postID, quality)
			if err := c.ensureVideoAsset(postFromFeed, quality, true, logger); err != nil {
				logger.Printf("Error downloading video for %s: %v", postID, err)
			}
			continue
		}

		// File exists locally, proceed with validation.
		shouldAdopt := false
		if quality == tikwm.AssetSD {
			// For SD, we can validate size first.
			if postFromFeed.Size > 0 && size == int64(postFromFeed.Size) {
				logger.Printf("Local SD file for post %s has correct size. Proceeding to ffmpeg validation.", postID)
				shouldAdopt = true
			} else {
				logger.Printf("Local SD file for post %s has incorrect size (expected: %d, actual: %d). Re-downloading.", postID, postFromFeed.Size, size)
			}
		} else { // For HD and Source, we must rely on ffmpeg validation alone.
			logger.Printf("Local %s file found for %s. Proceeding to ffmpeg validation.", quality, postID)
			shouldAdopt = true
		}

		if shouldAdopt && c.cfg.FfmpegPath != "" {
			valid, validationErr := validator(c.getAssetPath(postFromFeed, quality))
			if validationErr != nil {
				logger.Printf("Ffmpeg validation failed for %s (quality: %s): %v. Re-downloading.", postID, quality, validationErr)
				shouldAdopt = false
			} else if valid {
				logger.Printf("Ffmpeg validation passed for %s (quality: %s). Adopting.", postID, quality)
			}
		}

		if shouldAdopt {
			if err := c.adoptLocalAsset(postFromFeed, quality, logger); err != nil {
				logger.Printf("Failed to adopt existing file for %s (quality: %s): %v", postID, quality, err)
			}
		} else {
			// If we decided not to adopt for any reason (bad size, failed validation), re-download.
			if err := c.ensureVideoAsset(postFromFeed, quality, true, logger); err != nil {
				logger.Printf("Error re-downloading video for %s: %v", postID, err)
			}
		}
	}
}

// ensureVideoAsset handles the logic for making sure a video asset exists on disk and is recorded in the database.
func (c *Client) ensureVideoAsset(post *tikwm.Post, assetType tikwm.AssetType, force bool, logger *log.Logger) error {
	if !force {
		exists, err := c.db.AssetExists(post.ID(), assetType)
		if err != nil {
			return fmt.Errorf("db check failed for post %s, quality %s: %w", post.ID(), assetType, err)
		}
		if exists {
			logger.Printf("Asset for %s (quality: %s) already in database. Skipping.", post.ID(), assetType)
			return nil
		}
	}
	logger.Printf("Processing video asset for post %s (quality: %s)...", post.ID(), assetType)

	// Create a copy of the post for download logic to avoid potential mutation
	postCopy := *post

	// DownloadVideo now contains all the necessary logic to fetch URLs and retry.
	// It's called with the correct assetType, so it will produce the correct filename.
	_, sha, err := postCopy.DownloadVideo(assetType, tikwm.DownloadOpt{Directory: c.cfg.DownloadPath, FfmpegPath: c.cfg.FfmpegPath})
	if err != nil {
		// If a quality fails, we let it fail. The user can use `quality: all`
		// or the `fix` command to download other qualities as a form of fallback.
		return err
	}
	if sha == "" {
		return fmt.Errorf("asset processing succeeded but returned empty SHA256 hash for post %s", post.ID())
	}

	if err := c.db.AddOrUpdateAsset(post.ID(), post.Author.UniqueId, post.CreateTime, assetType, sha); err != nil {
		return err
	}
	// Save title after successful video download and DB update.
	return c.savePostTitle(post, logger)
}

func (c *Client) ensureCoverAsset(post *tikwm.Post, force bool, logger *log.Logger) error {
	assetType, coverURL := c.getCoverAssetType(post)
	if coverURL == "" {
		return fmt.Errorf("no URL found for configured cover type '%s' on post %s", c.cfg.CoverType, post.ID())
	}

	if !force {
		exists, err := c.db.AssetExists(post.ID(), assetType)
		if err != nil {
			return fmt.Errorf("db check failed for cover %s: %w", assetType, err)
		}
		if exists {
			return nil
		}
	}

	logger.Printf("Processing cover for post %s (type: %s)...", post.ID(), assetType)

	fullPath := c.getAssetPath(post, assetType)
	creatorDir := filepath.Dir(fullPath)
	// #nosec G301
	if err := os.MkdirAll(creatorDir, 0755); err != nil {
		return fmt.Errorf("failed to create creator directory %s: %w", creatorDir, err)
	}

	sha, err := tikwm.DownloadAndHash(coverURL, fullPath)
	if err != nil {
		return err
	}
	return c.db.AddOrUpdateAsset(post.ID(), post.Author.UniqueId, post.CreateTime, assetType, sha)
}

func (c *Client) processCoverInFeed(post *tikwm.Post, force bool, logger *log.Logger) {
	if !c.cfg.DownloadCovers {
		return
	}
	if err := c.ensureCoverAsset(post, force, logger); err != nil {
		logger.Printf("Could not process cover for post %s: %v", post.ID(), err)
	}
}

func (c *Client) processAlbumInFeed(post *tikwm.Post, force bool, logger *log.Logger, progressCb ProgressCallback, current, total int) {
	postID := post.ID()
	totalPhotosInAlbum := len(post.Images)
	if totalPhotosInAlbum == 0 {
		logger.Printf("Post %s is an album but has no images in feed data, skipping.", postID)
		return
	}

	if !force {
		countInDb, err := c.db.GetAlbumPhotoCount(postID)
		if err != nil {
			logger.Printf("DB check failed for album %s: %v", postID, err)
			return
		}
		if countInDb >= totalPhotosInAlbum {
			progressCb(current, total, fmt.Sprintf("Album %s complete", postID))
			logger.Printf("--- Album %s already complete in database. ---", postID)
			return
		}
	}

	// Album needs processing. Fetch full details to ensure data is fresh.
	logger.Printf("Album %s requires processing. Fetching full post details.", postID)
	finalPost, fetchErr := c.getPostWithRetry(post, progressCb, current, total)
	if fetchErr != nil {
		logger.Printf("Failed to get full post details for %s: %v", postID, fetchErr)
		return
	}

	if !finalPost.IsAlbum() || len(finalPost.Images) == 0 {
		logger.Printf("Post %s is not a valid album after fetching full details, skipping.", postID)
		return
	}

	if err := c.ensureAlbum(finalPost, force, logger); err != nil {
		logger.Printf("Error processing album for post %s: %v", postID, err)
	}
}

// ensureAlbum handles the logic for downloading all photos in an album and recording them in the database.
func (c *Client) ensureAlbum(post *tikwm.Post, force bool, logger *log.Logger) error {
	logger.Printf("Processing album for post %s (%d images)...", post.ID(), len(post.Images))

	// Migration: Delete old single-row entry for this album if it exists.
	if err := c.db.DeletePost(post.ID()); err != nil {
		// This is not a fatal error, as the post might not have existed before.
		logger.Printf("Note: Could not perform migration delete for post %s: %v", post.ID(), err)
	}

	for i := range post.Images {
		photoIndex := i   // 0-based for array access
		photoNum := i + 1 // 1-based for ID
		albumPhotoID := fmt.Sprintf("%s_%d_%d", post.ID(), photoNum, len(post.Images))

		if !force {
			exists, err := c.db.AssetExists(albumPhotoID, tikwm.AssetAlbumPhoto)
			if err != nil {
				logger.Printf("DB check failed for photo %s: %v. Skipping.", albumPhotoID, err)
				continue
			}
			if exists {
				logger.Printf("Photo %s already exists in database.", albumPhotoID)
				continue
			}
		}

		logger.Printf("Processing photo %d/%d for post %s.", photoNum, len(post.Images), post.ID())

		_, sha, err := post.DownloadAlbumPhoto(photoIndex, tikwm.DownloadOpt{Directory: c.cfg.DownloadPath})
		if err != nil {
			logger.Printf("Failed to download photo %s: %v", albumPhotoID, err)
			continue
		}
		if sha == "" {
			logger.Printf("Photo processing succeeded but returned empty SHA256 hash for %s", albumPhotoID)
			continue
		}

		err = c.db.AddOrUpdateAsset(albumPhotoID, post.Author.UniqueId, post.CreateTime, tikwm.AssetAlbumPhoto, sha)
		if err != nil {
			logger.Printf("Failed to add photo %s to database: %v", albumPhotoID, err)
		} else {
			logger.Printf("Successfully processed and stored photo %s", albumPhotoID)
		}
	}
	// Save title once after album is processed.
	return c.savePostTitle(post, logger)
}

// DownloadCoversForUser downloads missing covers for all posts by a user.
func (c *Client) DownloadCoversForUser(username string, logger *log.Logger, progressCb ProgressCallback) error {
	if progressCb == nil {
		progressCb = noOpProgress
	}
	posts, err := c.db.GetPostsByAuthor(username)
	if err != nil {
		return fmt.Errorf("failed to get posts from DB for %s: %w", username, err)
	}
	if len(posts) == 0 {
		logger.Printf("No posts found in database for %s. Download posts first.", username)
		return nil
	}
	for i, record := range posts {
		progressCb(i+1, len(posts), "Checking post "+record.ID)
		if record.HasCover {
			continue
		}
		// Album photo entries have composite IDs and won't be processed here, which is correct.
		if strings.Contains(record.ID, "_") {
			continue
		}
		post, err := tikwm.GetPost(record.ID, false)
		if err != nil {
			logger.Printf("Could not get post details for %s: %v", record.ID, err)
			continue
		}
		if err := c.ensureCoverAsset(post, false, logger); err != nil {
			logger.Printf("Could not download cover for post %s: %v", post.ID(), err)
		}
	}
	return nil
}

// FixProfile downloads videos for a user that are present in the database but are missing the desired asset.
func (c *Client) FixProfile(username string, logger *log.Logger, progressCb ProgressCallback) error {
	if progressCb == nil {
		progressCb = noOpProgress
	}
	qualities, err := c.getQualitiesToDownload()
	if err != nil {
		return err
	}
	for _, assetType := range qualities {
		progressCb(0, 0, fmt.Sprintf("Checking database for missing %s videos...", assetType))
		missingPosts, err := c.db.GetMissingPostsByAuthor(username, assetType)
		if err != nil {
			return fmt.Errorf("failed to get missing posts from DB for %s: %w", username, err)
		}
		if len(missingPosts) == 0 {
			progressCb(0, 0, fmt.Sprintf("No missing %s videos found for %s.", assetType, username))
			continue
		}
		progressCb(0, len(missingPosts), fmt.Sprintf("Found %d missing %s videos.", len(missingPosts), assetType))
		for i, record := range missingPosts {
			progressCb(i+1, len(missingPosts), "Processing "+record.ID)
			post, err := tikwm.GetPost(record.ID, true)
			if err != nil {
				logger.Printf("Could not get post details for %s: %v", record.ID, err)
				continue
			}
			if err := c.ensureVideoAsset(post, assetType, true, logger); err != nil {
				logger.Printf("Failed to process video for post %s (quality: %s): %v", post.ID(), assetType, err)
			}
		}
	}
	return nil
}

func (c *Client) getPostWithRetry(post *tikwm.Post, progressCb ProgressCallback, current, total int) (*tikwm.Post, error) {
	maxRetries := 5
	for i := 0; i < maxRetries; i++ {
		progressCb(current, total, "Fetching post details for "+post.ID())
		hdPost, err := tikwm.GetPost(post.ID(), true)
		if err != nil {
			if strings.Contains(err.Error(), "(429)") {
				if !c.cfg.RetryOn429 {
					return nil, fmt.Errorf("rate limited (429) fetching post %s, aborting. Enable --retry-on-429 to retry", post.ID())
				}
				wait := time.Second * time.Duration(1<<i)
				progressCb(current, total, fmt.Sprintf("Rate limited. Retrying in %s...", wait))
				time.Sleep(wait)
				continue
			}
			return nil, err
		}
		return hdPost, nil
	}
	return nil, fmt.Errorf("failed to get details for %s after %d retries", post.ID(), maxRetries)
}
