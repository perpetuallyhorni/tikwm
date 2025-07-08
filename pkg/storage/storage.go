package storage

import (
	tikwm "github.com/perpetuallyhorni/tikwm/internal"
)

// PostRecord represents a single row from the posts table.
type PostRecord struct {
	// ID is the unique identifier for the post.
	ID string
	// AuthorID is the identifier of the post's author.
	AuthorID string
	// CreateTime is the post's creation timestamp in Unix epoch seconds.
	CreateTime int64
	// HasCover indicates whether the post has a cover image.
	HasCover bool
}

// Storer defines the interface for database operations.
// This allows for different database backends to be used with the client.
type Storer interface {
	// AddOrUpdateAsset adds or updates a generic asset record for a post.
	AddOrUpdateAsset(postID, authorID string, createTime int64, assetType tikwm.AssetType, sha256 string) error
	// AssetExists checks if a specific asset for a post exists in the database.
	AssetExists(assetID string, assetType tikwm.AssetType) (bool, error)
	// GetAlbumPhotoCount retrieves the number of downloaded photos for a given album post ID.
	GetAlbumPhotoCount(postID string) (int, error)
	// DeletePost deletes a post record by its exact ID. Used for migrating old album entries.
	DeletePost(postID string) error
	// AddAvatar adds a record for a downloaded user avatar.
	AddAvatar(authorID, sha256 string) error
	// AvatarExists checks if a specific avatar hash for a user already exists.
	AvatarExists(authorID, sha256 string) (bool, error)
	// GetPostsByAuthor retrieves all post records for a given author.
	GetPostsByAuthor(authorID string) ([]PostRecord, error)
	// GetMissingPostsByAuthor retrieves post records for an author that are missing a specific asset type.
	GetMissingPostsByAuthor(authorID string, assetType tikwm.AssetType) ([]PostRecord, error)
	// Close closes the database connection.
	Close() error
}
