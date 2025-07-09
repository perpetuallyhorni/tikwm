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
