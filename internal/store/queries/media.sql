-- name: GetMedia :one
SELECT key, kind, content_type, bytes, width, height, variants, created_at, created_by
FROM media
WHERE key = @key;

-- name: ListMediaByKeys :many
SELECT key, kind, content_type, bytes, width, height, variants, created_at, created_by
FROM media
WHERE key = ANY(@keys::text[]);

-- name: InsertMedia :exec
-- Content-addressed: inserting the same key twice is a no-op.
INSERT INTO media (key, kind, content_type, bytes, width, height, variants, created_by)
VALUES (@key, @kind, @content_type, @bytes, @width, @height, @variants, @created_by)
ON CONFLICT (key) DO NOTHING;
