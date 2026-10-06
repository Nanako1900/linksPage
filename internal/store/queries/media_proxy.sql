-- name: UpsertMediaProxy :exec
-- Registers an upstream URL; re-registering refreshes last_seen_at.
INSERT INTO media_proxy (key, provider, url, ext, kind)
VALUES (@key, @provider, @url, @ext, @kind)
ON CONFLICT (key) DO UPDATE SET last_seen_at = now();

-- name: GetMediaProxy :one
SELECT key, provider, url, ext, kind, created_at, last_seen_at
FROM media_proxy
WHERE key = @key;

-- name: TouchMediaProxy :exec
UPDATE media_proxy SET last_seen_at = now()
WHERE key = ANY(@keys::text[]);
