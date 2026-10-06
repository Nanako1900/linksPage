-- name: ListCommunities :many
SELECT id, page_id, slug, provider, platform, external_id, config, display, icon_key,
       invite_url, fallback_url, refresh_interval, created_at, updated_at
FROM communities
WHERE page_id = @page_id
ORDER BY created_at, id;

-- name: GetCommunityBySlug :one
SELECT id, page_id, slug, provider, platform, external_id, config, display, icon_key,
       invite_url, fallback_url, refresh_interval, created_at, updated_at
FROM communities
WHERE slug = @slug;

-- name: GetGoCommunity :one
-- Everything /go/{slug} needs to pick a target for a community (doc 5.6).
-- Only communities published on the page right now: hidden, scheduled,
-- expired or unplaced ones resolve like unknown slugs.
SELECT c.id, c.slug, c.provider, c.platform, c.display, c.invite_url, c.fallback_url,
       s.state AS snapshot_state, s.err_code AS snapshot_err_code, s.data AS snapshot_data,
       q.id AS qr_id
FROM communities c
LEFT JOIN provider_snapshots s ON s.community_id = c.id
LEFT JOIN community_qr_codes q ON q.community_id = c.id
WHERE c.slug = @slug
  AND EXISTS (
      SELECT 1 FROM page_blocks b
      WHERE b.page_id = c.page_id
        AND b.community_id = c.id
        AND b.visible
        AND (b.visible_from IS NULL OR b.visible_from <= now())
        AND (b.visible_to IS NULL OR b.visible_to > now())
  );

-- name: ListDueCommunities :many
-- Provider-backed communities whose next fetch is due (or that were never
-- fetched), most overdue first.
SELECT c.id, c.slug, c.provider, c.external_id, c.config, c.invite_url, c.refresh_interval,
       s.fail_count AS fail_count, s.last_ok_at AS last_ok_at
FROM communities c
LEFT JOIN provider_snapshots s ON s.community_id = c.id
WHERE c.provider <> 'static'
  AND (s.next_fetch_at IS NULL OR s.next_fetch_at <= @now::timestamptz)
ORDER BY s.next_fetch_at NULLS FIRST, c.id
LIMIT @max_rows;

-- name: InsertCommunity :one
INSERT INTO communities (page_id, slug, provider, platform, external_id, config, display, icon_key,
                         invite_url, fallback_url, refresh_interval)
VALUES (@page_id, @slug, @provider, @platform, @external_id, @config, @display, @icon_key,
        @invite_url, @fallback_url, @refresh_interval)
RETURNING id;
