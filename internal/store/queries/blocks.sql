-- name: ListVisibleBlocks :many
-- Blocks visible at @now, in display order.
SELECT id, page_id, kind, community_id, link_id, data, sort_order, visible, visible_from, visible_to
FROM page_blocks
WHERE page_id = @page_id
  AND visible
  AND (visible_from IS NULL OR visible_from <= @now::timestamptz)
  AND (visible_to IS NULL OR visible_to > @now::timestamptz)
ORDER BY sort_order, id;

-- name: NextBlockBoundary :one
-- The earliest future visible_from/visible_to after @now (NULL when none):
-- the public page must be rebuilt when it is reached (doc 4.6).
SELECT min(b)::timestamptz AS next_boundary
FROM (
    SELECT f.visible_from AS b FROM page_blocks f
    WHERE f.page_id = sqlc.arg(page_id)::smallint AND f.visible AND f.visible_from > sqlc.arg(now)::timestamptz
    UNION ALL
    SELECT t.visible_to AS b FROM page_blocks t
    WHERE t.page_id = sqlc.arg(page_id)::smallint AND t.visible AND t.visible_to > sqlc.arg(now)::timestamptz
) AS boundaries;

-- name: InsertBlock :one
INSERT INTO page_blocks (page_id, kind, community_id, link_id, data, sort_order, visible, visible_from, visible_to)
VALUES (@page_id, @kind, @community_id, @link_id, @data, @sort_order, @visible, @visible_from, @visible_to)
RETURNING id;
