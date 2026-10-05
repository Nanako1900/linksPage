-- name: ListLinks :many
SELECT id, page_id, slug, kind, label, url, icon, icon_key, rel_me, created_at, updated_at
FROM links
WHERE page_id = @page_id
ORDER BY created_at, id;

-- name: GetLinkBySlug :one
SELECT id, page_id, slug, kind, label, url, icon, icon_key, rel_me, created_at, updated_at
FROM links
WHERE slug = @slug;

-- name: GetGoLink :one
-- The /go/{slug} target of a link published on the page right now (a
-- visible link block, or a visible social_row listing it); hidden,
-- scheduled, expired or unplaced links resolve like unknown slugs.
SELECT l.id, l.slug, l.url
FROM links l
WHERE l.slug = @slug
  AND EXISTS (
      SELECT 1 FROM page_blocks b
      WHERE b.page_id = l.page_id
        AND b.visible
        AND (b.visible_from IS NULL OR b.visible_from <= now())
        AND (b.visible_to IS NULL OR b.visible_to > now())
        AND (b.link_id = l.id
             OR (b.kind = 'social_row' AND b.data -> 'linkIds' @> jsonb_build_array(l.id::text)))
  );

-- name: InsertLink :one
INSERT INTO links (page_id, slug, kind, label, url, icon, icon_key, rel_me)
VALUES (@page_id, @slug, @kind, @label, @url, @icon, @icon_key, @rel_me)
RETURNING id;
