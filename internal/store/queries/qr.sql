-- name: GetQRCode :one
-- Served by /media/q/{id}; a missing id means replaced or deleted (410).
SELECT q.id, q.community_id, q.media_key, q.note, q.updated_at,
       m.content_type, m.bytes, m.width, m.height
FROM community_qr_codes q
JOIN media m ON m.key = q.media_key
WHERE q.id = @id;

-- name: ListQRCodes :many
SELECT q.id, q.community_id, q.media_key, q.note, q.updated_at,
       m.content_type, m.width, m.height
FROM community_qr_codes q
JOIN communities c ON c.id = q.community_id
JOIN media m ON m.key = q.media_key
WHERE c.page_id = @page_id;

-- name: InsertQRCode :one
INSERT INTO community_qr_codes (community_id, media_key, note)
VALUES (@community_id, @media_key, @note)
RETURNING id;
