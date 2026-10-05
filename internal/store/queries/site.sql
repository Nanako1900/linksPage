-- name: GetSiteSettings :one
SELECT page_id, version, data, updated_at, updated_by
FROM site_settings
WHERE page_id = 1;

-- name: GetDefaultPage :one
SELECT id, slug
FROM pages
WHERE id = 1;

-- name: GetPageBySlug :one
SELECT id, slug
FROM pages
WHERE slug = $1;

-- name: GetMinCompatibleAppVersion :one
SELECT min_compatible_app_version
FROM schema_meta
WHERE id;
