SELECT id, author_id, create_time, (has_cover_medium OR has_cover_origin OR has_cover_dynamic) as has_cover
FROM posts
WHERE author_id = ?
ORDER BY create_time DESC;
