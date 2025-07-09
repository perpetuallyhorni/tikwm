INSERT INTO posts (id, author_id, create_time, {{.HasColumn}}, {{.ShaColumn}}, downloaded_at)
VALUES (?, ?, ?, 1, ?, ?)
ON CONFLICT(id) DO UPDATE SET
    {{.HasColumn}} = 1,
    {{.ShaColumn}} = excluded.{{.ShaColumn}},
    downloaded_at = excluded.downloaded_at;
