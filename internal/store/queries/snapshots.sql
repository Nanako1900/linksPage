-- name: ListProviderSnapshots :many
SELECT s.community_id, s.data, s.state, s.err_code, s.fetched_at, s.last_ok_at, s.next_fetch_at, s.fail_count
FROM provider_snapshots s
JOIN communities c ON c.id = s.community_id
WHERE c.page_id = @page_id;

-- name: GetProviderSnapshot :one
SELECT community_id, data, state, err_code, fetched_at, last_ok_at, next_fetch_at, fail_count
FROM provider_snapshots
WHERE community_id = @community_id;

-- name: UpsertProviderSnapshot :exec
INSERT INTO provider_snapshots (community_id, data, state, err_code, fetched_at, last_ok_at, next_fetch_at, fail_count)
VALUES (@community_id, @data, @state, @err_code, @fetched_at, @last_ok_at, @next_fetch_at, @fail_count)
ON CONFLICT (community_id) DO UPDATE SET
    data          = excluded.data,
    state         = excluded.state,
    err_code      = excluded.err_code,
    fetched_at    = excluded.fetched_at,
    last_ok_at    = excluded.last_ok_at,
    next_fetch_at = excluded.next_fetch_at,
    fail_count    = excluded.fail_count;

-- name: EnsureProviderSnapshot :exec
-- Creates a pending row so a new community is fetched right away.
INSERT INTO provider_snapshots (community_id, state, next_fetch_at)
VALUES (@community_id, 'pending', now())
ON CONFLICT (community_id) DO NOTHING;
