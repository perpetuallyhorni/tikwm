package sqlite

import (
	"bytes"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"text/template"
	"time"

	_ "github.com/mattn/go-sqlite3"
	tikwm "github.com/perpetuallyhorni/tikwm/internal"
	"github.com/perpetuallyhorni/tikwm/pkg/storage"
)

//go:embed queries/*.sql
//go:embed queries/*.sql.tpl
var queryFS embed.FS

// DB is a SQLite implementation of the storage.Storer interface.
type DB struct {
	Conn *sql.DB // The raw database connection, exposed for extensibility.
}

// New creates a new SQLite database connection and ensures the schema is up to date.
// It returns a concrete *DB type to allow for extension.
func New(path string) (*DB, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	db, err := sql.Open("sqlite3", fmt.Sprintf("file:%s?_journal_mode=WAL", path))
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	instance := &DB{Conn: db}
	if err := instance.createSchema(); err != nil {
		_ = instance.Close()
		return nil, fmt.Errorf("failed to create database schema: %w", err)
	}

	return instance, nil
}

// getQuery reads a raw SQL query from the embedded filesystem.
func getQuery(name string) (string, error) {
	b, err := queryFS.ReadFile("queries/" + name)
	if err != nil {
		return "", fmt.Errorf("failed to read embedded query %s: %w", name, err)
	}
	return string(b), nil
}

// getParsedQuery parses and executes a SQL template from the embedded filesystem.
func getParsedQuery(templateName string, data any) (string, error) {
	t, err := template.ParseFS(queryFS, "queries/"+templateName)
	if err != nil {
		return "", fmt.Errorf("failed to parse embedded query template %s: %w", templateName, err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute embedded query template %s: %w", templateName, err)
	}
	return buf.String(), nil
}

// createSchema creates the necessary tables in the SQLite database if they don't exist.
func (db *DB) createSchema() error {
	query, err := getQuery("schema.sql")
	if err != nil {
		return err
	}
	_, err = db.Conn.Exec(query)
	return err
}

// AddAvatar adds a record for a downloaded user avatar.
func (db *DB) AddAvatar(authorID, sha256 string) error {
	query, err := getQuery("add_avatar.sql")
	if err != nil {
		return err
	}
	_, err = db.Conn.Exec(query, authorID, sha256, time.Now())
	if err != nil {
		return fmt.Errorf("failed to insert avatar for author %s: %w", authorID, err)
	}
	return nil
}

// AvatarExists checks if a specific avatar hash for an author is already in the database.
func (db *DB) AvatarExists(authorID, sha256 string) (bool, error) {
	query, err := getQuery("avatar_exists.sql")
	if err != nil {
		return false, err
	}
	var exists bool
	err = db.Conn.QueryRow(query, authorID, sha256).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check if avatar exists for author %s: %w", authorID, err)
	}
	return exists, nil
}

// AddOrUpdateAsset uses an atomic "upsert" for a generic asset and forces a WAL checkpoint.
func (db *DB) AddOrUpdateAsset(postID, authorID string, createTime int64, assetType tikwm.AssetType, sha256 string) error {
	var hasColumn, shaColumn string
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
		hasColumn, shaColumn = "has_hd", "sha256_hd"
	default:
		return fmt.Errorf("unknown asset type for DB operation: %s", assetType)
	}

	query, err := getParsedQuery("upsert_asset.sql.tpl", struct {
		HasColumn string
		ShaColumn string
	}{
		HasColumn: hasColumn,
		ShaColumn: shaColumn,
	})
	if err != nil {
		return err
	}

	_, err = db.Conn.Exec(query, postID, authorID, createTime, sha256, time.Now())
	if err != nil {
		return fmt.Errorf("failed to execute upsert for post %s (type: %s): %w", postID, assetType, err)
	}

	_, err = db.Conn.Exec("PRAGMA wal_checkpoint(TRUNCATE);")
	if err != nil {
		return fmt.Errorf("failed to checkpoint WAL after upsert for post %s: %w", postID, err)
	}
	return nil
}

// AssetExists checks if a specific asset for a post exists in the database.
func (db *DB) AssetExists(assetID string, assetType tikwm.AssetType) (bool, error) {
	var column string
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
		column = "has_hd"
	default:
		return false, fmt.Errorf("unknown asset type for existence check: %s", assetType)
	}

	query, err := getParsedQuery("asset_exists.sql.tpl", struct{ Column string }{Column: column})
	if err != nil {
		return false, err
	}
	var exists bool
	err = db.Conn.QueryRow(query, assetID).Scan(&exists)
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
	query, err := getQuery("count_album_photos.sql")
	if err != nil {
		return 0, err
	}
	pattern := postID + `_%`
	var count int
	err = db.Conn.QueryRow(query, pattern).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count album photos for %s: %w", postID, err)
	}
	return count, nil
}

// DeletePost deletes a post record by its exact ID.
func (db *DB) DeletePost(postID string) error {
	query, err := getQuery("delete_post.sql")
	if err != nil {
		return err
	}
	_, err = db.Conn.Exec(query, postID)
	if err != nil {
		return fmt.Errorf("failed to delete post %s: %w", postID, err)
	}
	return nil
}

// GetPostsByAuthor retrieves all post records for a given author from the database.
func (db *DB) GetPostsByAuthor(authorID string) ([]storage.PostRecord, error) {
	query, err := getQuery("get_posts_by_author.sql")
	if err != nil {
		return nil, err
	}
	rows, err := db.Conn.Query(query, authorID)
	if err != nil {
		return nil, fmt.Errorf("failed to query posts for author %s: %w", authorID, err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			fmt.Printf("failed to close rows: %v", err)
		}
	}()

	var posts []storage.PostRecord
	for rows.Next() {
		var p storage.PostRecord
		if err := rows.Scan(&p.ID, &p.AuthorID, &p.CreateTime, &p.HasCover); err != nil {
			return nil, fmt.Errorf("failed to scan post row: %w", err)
		}
		posts = append(posts, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error during row iteration for author %s: %w", authorID, err)
	}
	return posts, nil
}

// GetMissingPostsByAuthor retrieves post records that are missing a specific asset type.
func (db *DB) GetMissingPostsByAuthor(authorID string, assetType tikwm.AssetType) ([]storage.PostRecord, error) {
	var hasColumn string
	switch assetType {
	case tikwm.AssetHD:
		hasColumn = "has_hd"
	case tikwm.AssetSD:
		hasColumn = "has_sd"
	default:
		return nil, fmt.Errorf("unsupported asset type for fix: %s", assetType)
	}
	query, err := getParsedQuery("get_missing_posts_by_author.sql.tpl", struct{ HasColumn string }{HasColumn: hasColumn})
	if err != nil {
		return nil, err
	}
	rows, err := db.Conn.Query(query, authorID)
	if err != nil {
		return nil, fmt.Errorf("failed to query missing posts for author %s: %w", authorID, err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			fmt.Printf("failed to close rows: %v", err)
		}
	}()

	var posts []storage.PostRecord
	for rows.Next() {
		var p storage.PostRecord
		if err := rows.Scan(&p.ID, &p.AuthorID, &p.CreateTime, &p.HasCover); err != nil {
			return nil, fmt.Errorf("failed to scan post row: %w", err)
		}
		posts = append(posts, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error during row iteration for author %s: %w", authorID, err)
	}
	return posts, nil
}

// Close closes the database connection.
func (db *DB) Close() error {
	return db.Conn.Close()
}
