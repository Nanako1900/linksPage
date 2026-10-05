-- name: IsContentEmpty :one
-- Seeding only runs on an empty page (no blocks, communities or links).
SELECT (
    NOT EXISTS (SELECT 1 FROM page_blocks)
    AND NOT EXISTS (SELECT 1 FROM communities)
    AND NOT EXISTS (SELECT 1 FROM links)
)::boolean AS empty;

-- name: ReplaceSiteSettingsData :one
-- Writes seeded settings and bumps the version.
UPDATE site_settings
SET data = @data, version = version + 1, updated_at = now(), updated_by = @updated_by
WHERE page_id = @page_id
RETURNING version;
