SELECT EXISTS(SELECT 1 FROM avatars WHERE author_id = ? AND sha256 = ?);
