package sqlite

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
	tikwm "github.com/perpetuallyhorni/tikwm/internal"
	"github.com/perpetuallyhorni/tikwm/pkg/storage"
)

// DB is a SQLite implementation of the storage.Storer interface.
type DB struct {
	*sql.DB // Embed the standard sql.DB type.
}

// New creates a new SQLite database connection and ensures the schema is up to date.
func New(path string) (storage.Storer, error) {
	// Determine the directory for the database file.
	dir := filepath.Dir(path)
	// Create the directory if it doesn't exist.
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}
	// Use WAL mode for better concurrency, but manage it carefully.
	db, err := sql.Open("sqlite3", fmt.Sprintf("file:%s?_journal_mode=WAL", path))
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	// Ensure the connection is valid by pinging the database.
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}
	// Create the database schema if it doesn't exist.
	if err := createSchema(db); err != nil {
		return nil, fmt.Errorf("failed to create database schema: %w", err)
	}
	// Return a new DB instance wrapping the sql.DB connection.
	return &DB{db}, nil
}

// createSchema creates the necessary tables in the SQLite database if they don't exist.
func createSchema(db *sql.DB) error {
	query := `
	CREATE TABLE IF NOT EXISTS posts (
		id TEXT PRIMARY KEY,
		author_id TEXT NOT NULL,
		create_time INTEGER NOT NULL,
		has_sd BOOLEAN NOT NULL DEFAULT 0,
		has_hd BOOLEAN NOT NULL DEFAULT 0,
		has_source BOOLEAN NOT NULL DEFAULT 0,
		has_cover_medium BOOLEAN NOT NULL DEFAULT 0,
		has_cover_origin BOOLEAN NOT NULL DEFAULT 0,
		has_cover_dynamic BOOLEAN NOT NULL DEFAULT 0,
		sha256_sd TEXT,
		sha256_hd TEXT,
		sha256_source TEXT,
		sha256_cover_medium TEXT,
		sha256_cover_origin TEXT,
		sha256_cover_dynamic TEXT,
		downloaded_at TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_author_id_create_time ON posts (author_id, create_time);
	
	CREATE TABLE IF NOT EXISTS avatars (
		author_id TEXT NOT NULL,
		sha256 TEXT NOT NULL,
		downloaded_at TIMESTAMP NOT NULL,
		PRIMARY KEY (author_id, sha256)
	);
	`
	_, err := db.Exec(query)
	return err
}

// AddAvatar adds a record for a downloaded user avatar.
func (db *DB) AddAvatar(authorID, sha256 string) error {
	query := `INSERT OR IGNORE INTO avatars (author_id, sha256, downloaded_at) VALUES (?, ?, ?)`
	_, err := db.Exec(query, authorID, sha256, time.Now())
	if err != nil {
		return fmt.Errorf("failed to insert avatar for author %s: %w", authorID, err)
	}
	return nil
}

// AvatarExists checks if a specific avatar hash for an author is already in the database.
func (db *DB) AvatarExists(authorID, sha256 string) (bool, error) {
	var exists bool
	query := `SELECT EXISTS(SELECT 1 FROM avatars WHERE author_id = ? AND sha256 = ?)`
	err := db.QueryRow(query, authorID, sha256).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check if avatar exists for author %s: %w", authorID, err)
	}
	return exists, nil
}

// AddOrUpdateAsset uses an atomic "upsert" for a generic asset and forces a WAL checkpoint.
func (db *DB) AddOrUpdateAsset(postID, authorID string, createTime int64, assetType tikwm.AssetType, sha256 string) error {
	var hasColumn, shaColumn string
	// Determine the column names based on the asset type.
	switch assetType {
	case tikwm.AssetSD:
		hasColumn, shaColumn = "has_sd", "sha256_sd"
	case tikwm.AssetHD:
		hasColumn, shaColumn = "has_hd", "sha256_hd"
	case tikwm.AssetSource:
		hasColumn, shaColumn = "has_source", "sha256_source"
	case tikwm.AssetCoverMedium:
		hasColumn, shaColumn = "has_cover_medium", "sha256_cover_medium"
	case tikwm.AssetCoverOrigin:
		hasColumn, shaColumn = "has_cover_origin", "sha256_cover_origin"
	case tikwm.AssetCoverDynamic:
		hasColumn, shaColumn = "has_cover_dynamic", "sha256_cover_dynamic"
	case tikwm.AssetAlbumPhoto:
		// Album photos are simple, they just use the HD columns for presence.
		hasColumn, shaColumn = "has_hd", "sha256_hd"
	default:
		return fmt.Errorf("unknown asset type for DB operation: %s", assetType)
	}

	// Construct the SQL query for inserting or updating the asset.
	query := fmt.Sprintf(`
		INSERT INTO posts (id, author_id, create_time, %[1]s, %[2]s, downloaded_at)
		VALUES (?, ?, ?, 1, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			%[1]s = 1,
			%[2]s = excluded.%[2]s,
			downloaded_at = excluded.downloaded_at;
	`, hasColumn, shaColumn)

	_, err := db.Exec(query, postID, authorID, createTime, sha256, time.Now())
	if err != nil {
		return fmt.Errorf("failed to execute upsert for post %s (type: %s): %w", postID, assetType, err)
	}

	// Force a WAL checkpoint to ensure data is written to disk.
	_, err = db.Exec("PRAGMA wal_checkpoint(TRUNCATE);")
	if err != nil {
		return fmt.Errorf("failed to checkpoint WAL after upsert for post %s: %w", postID, err)
	}

	return nil
}

// AssetExists checks if a specific asset for a post exists in the database.
func (db *DB) AssetExists(assetID string, assetType tikwm.AssetType) (bool, error) {
	var column string
	// Determine the column name based on the asset type.
	switch assetType {
	case tikwm.AssetSD:
		column = "has_sd"
	case tikwm.AssetHD:
		column = "has_hd"
	case tikwm.AssetSource:
		column = "has_source"
	case tikwm.AssetCoverMedium:
		column = "has_cover_medium"
	case tikwm.AssetCoverOrigin:
		column = "has_cover_origin"
	case tikwm.AssetCoverDynamic:
		column = "has_cover_dynamic"
	case tikwm.AssetAlbumPhoto:
		column = "has_hd" // piggy-backing on hd for simple existence check
	default:
		return false, fmt.Errorf("unknown asset type for existence check: %s", assetType)
	}

	query := fmt.Sprintf("SELECT %s FROM posts WHERE id = ?", column)
	var exists bool
	err := db.QueryRow(query, assetID).Scan(&exists)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("failed to check if asset %s exists: %w", assetID, err)
	}
	return exists, nil
}

// GetAlbumPhotoCount retrieves the number of downloaded photos for an album.
func (db *DB) GetAlbumPhotoCount(postID string) (int, error) {
	query := `SELECT count(*) FROM posts WHERE id LIKE ?`
	// The post ID is numeric, so an underscore is a safe separator.
	pattern := postID + `_%`
	var count int
	err := db.QueryRow(query, pattern).Scan(&count)
	if err != nil {
		// count(*) doesn't return sql.ErrNoRows, just 0. So any error is a real problem.
		return 0, fmt.Errorf("failed to count album photos for %s: %w", postID, err)
	}
	return count, nil
}

// DeletePost deletes a post record by its exact ID.
func (db *DB) DeletePost(postID string) error {
	query := `DELETE FROM posts WHERE id = ?`
	_, err := db.Exec(query, postID)
	if err != nil {
		return fmt.Errorf("failed to delete post %s: %w", postID, err)
	}
	return nil
}

// GetPostsByAuthor retrieves all post records for a given author from the database.
func (db *DB) GetPostsByAuthor(authorID string) ([]storage.PostRecord, error) {
	query := `SELECT id, author_id, create_time, has_cover FROM posts WHERE author_id = ? ORDER BY create_time DESC`
	rows, err := db.Query(query, authorID)
	if err != nil {
		return nil, fmt.Errorf("failed to query posts for author %s: %w", authorID, err)
	}
	// Ensure rows are closed after function completes.
	defer func() {
		if err := rows.Close(); err != nil {
			fmt.Printf("failed to close rows: %v", err)
		}
	}()

	var posts []storage.PostRecord
	// Iterate through the rows returned by the query.
	for rows.Next() {
		var p storage.PostRecord
		// Scan the values from the row into the PostRecord struct.
		if err := rows.Scan(&p.ID, &p.AuthorID, &p.CreateTime, &p.HasCover); err != nil {
			return nil, fmt.Errorf("failed to scan post row: %w", err)
		}
		// Append the PostRecord to the slice of posts.
		posts = append(posts, p)
	}

	// Check for errors during row iteration.
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error during row iteration for author %s: %w", authorID, err)
	}

	return posts, nil
}

// GetMissingPostsByAuthor retrieves all post records for a given author from the database
// that are missing a specific asset type (hd or sd).
func (db *DB) GetMissingPostsByAuthor(authorID string, assetType tikwm.AssetType) ([]storage.PostRecord, error) {
	var query string
	// Exclude album photos from fix command by checking for underscores in the ID,
	// since base post IDs are numeric and do not contain them.
	const albumExclusion = `AND id NOT LIKE '%_%'`
	switch assetType {
	case tikwm.AssetHD:
		query = `SELECT id, author_id, create_time, has_cover FROM posts WHERE author_id = ? AND has_hd = 0 ` + albumExclusion + ` ORDER BY create_time DESC`
	case tikwm.AssetSD:
		query = `SELECT id, author_id, create_time, has_cover FROM posts WHERE author_id = ? AND has_sd = 0 ` + albumExclusion + ` ORDER BY create_time DESC`
	default:
		return nil, fmt.Errorf("unsupported asset type for fix: %s", assetType)
	}

	rows, err := db.Query(query, authorID)
	if err != nil {
		return nil, fmt.Errorf("failed to query missing posts for author %s: %w", authorID, err)
	}
	// Ensure rows are closed after function completes.
	defer func() {
		if err := rows.Close(); err != nil {
			fmt.Printf("failed to close rows: %v", err)
		}
	}()

	var posts []storage.PostRecord
	// Iterate through the rows returned by the query.
	for rows.Next() {
		var p storage.PostRecord
		// Scan the values from the row into the PostRecord struct.
		if err := rows.Scan(&p.ID, &p.AuthorID, &p.CreateTime, &p.HasCover); err != nil {
			return nil, fmt.Errorf("failed to scan post row: %w", err)
		}
		// Append the PostRecord to the slice of posts.
		posts = append(posts, p)
	}

	// Check for errors during row iteration.
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error during row iteration for author %s: %w", authorID, err)
	}

	return posts, nil
}

// Close closes the database connection.
func (db *DB) Close() error {
	return db.DB.Close()
}
