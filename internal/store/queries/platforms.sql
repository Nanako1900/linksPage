-- name: ListCustomPlatforms :many
SELECT id, name, icon, icon_key, url_pattern, needs_external_browser, created_at, updated_at
FROM custom_platforms
ORDER BY id;

-- name: InsertCustomPlatform :exec
INSERT INTO custom_platforms (id, name, icon, icon_key, url_pattern, needs_external_browser)
VALUES (@id, @name, @icon, @icon_key, @url_pattern, @needs_external_browser);
