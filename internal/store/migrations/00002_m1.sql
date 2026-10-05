-- +goose Up
-- M1 public page content (expand-only). Auth, analytics and presence tables
-- arrive in later milestones.

-- Uploaded and generated images. Content-addressed: key is the first 16
-- bytes of the sha256 of the stored bytes (32 lowercase hex chars) plus the
-- extension. Every stored file has its own row; "variants" on the primary
-- row maps a format to the key of a sibling row (e.g. {"png": "<key>.png"}).
CREATE TABLE media (
    key          text        PRIMARY KEY CHECK (key ~ '^[0-9a-f]{32}\.(webp|png|jpg)$'),
    kind         text        NOT NULL CHECK (kind IN ('avatar', 'icon', 'qr', 'og', 'favicon', 'background')),
    content_type text        NOT NULL CHECK (content_type IN ('image/webp', 'image/png', 'image/jpeg')),
    bytes        integer     NOT NULL CHECK (bytes > 0),
    width        integer     NOT NULL CHECK (width BETWEEN 1 AND 4096),
    height       integer     NOT NULL CHECK (height BETWEEN 1 AND 4096),
    variants     jsonb       NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(variants) = 'object'),
    created_at   timestamptz NOT NULL DEFAULT now(),
    created_by   text,
    CHECK (
        (content_type = 'image/webp' AND key LIKE '%.webp') OR
        (content_type = 'image/png' AND key LIKE '%.png') OR
        (content_type = 'image/jpeg' AND key LIKE '%.jpg')
    )
);

-- Registered upstream image URLs served by /media/p/{key}.{ext}. Unknown
-- keys are 404 without contacting the upstream (doc 5.2).
CREATE TABLE media_proxy (
    key          text        PRIMARY KEY CHECK (key ~ '^[A-Za-z0-9_-]{22}$'),
    provider     text        NOT NULL CHECK (provider IN ('discord', 'kook')),
    url          text        NOT NULL CHECK (url ~ '^https://' AND length(url) <= 2048),
    ext          text        NOT NULL CHECK (ext IN ('png', 'jpg', 'webp', 'gif')),
    kind         text        NOT NULL CHECK (kind IN ('icon', 'banner', 'splash', 'avatar')),
    created_at   timestamptz NOT NULL DEFAULT now(),
    last_seen_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX media_proxy_last_seen_idx ON media_proxy (last_seen_at);

-- Admin-defined platforms in addition to the embedded platforms.yaml
-- presets. Ids must not collide with preset ids (checked in Go).
CREATE TABLE custom_platforms (
    id                     text        PRIMARY KEY CHECK (id ~ '^[a-z][a-z0-9-]{1,31}$'),
    name                   jsonb       NOT NULL CHECK (jsonb_typeof(name) = 'object' AND name <> '{}'::jsonb),
    icon                   text        CHECK (icon ~ '^(si|builtin):[a-z0-9-]{1,64}$'),
    icon_key               text        REFERENCES media (key) ON DELETE SET NULL,
    url_pattern            text        CHECK (length(url_pattern) BETWEEN 1 AND 512),
    needs_external_browser boolean     NOT NULL DEFAULT false,
    created_at             timestamptz NOT NULL DEFAULT now(),
    updated_at             timestamptz NOT NULL DEFAULT now(),
    CHECK (icon IS NULL OR icon_key IS NULL)
);

-- Communities. provider decides how live data is fetched; platform is a
-- preset or custom platform id and decides the card variant.
--   discord: external_id = guild id, invite_url = permanent invite
--            (https://discord.gg/<code>)
--   kook:    external_id = guild id, invite_url = https://kook.top/<code>
--   static:  no external_id; QQ groups keep the official join link in
--            invite_url (QQ hosts only, checked in Go)
-- display holds the CommunityDisplay JSON (internal/site), including the
-- required localized name, QQ group number and the fallback contact.
CREATE TABLE communities (
    id               uuid        PRIMARY KEY DEFAULT uuidv7(),
    page_id          smallint    NOT NULL DEFAULT 1 REFERENCES pages (id) ON DELETE CASCADE,
    slug             text        NOT NULL UNIQUE CHECK (slug ~ '^[a-z0-9][a-z0-9-]{0,63}$'),
    provider         text        NOT NULL CHECK (provider IN ('discord', 'kook', 'static')),
    platform         text        NOT NULL CHECK (platform ~ '^[a-z][a-z0-9-]{1,31}$'),
    external_id      text,
    config           jsonb       NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(config) = 'object'),
    display          jsonb       NOT NULL CHECK (
        jsonb_typeof(display) = 'object'
        AND jsonb_typeof(display -> 'name') = 'object'
        AND display -> 'name' <> '{}'::jsonb
    ),
    icon_key         text        REFERENCES media (key) ON DELETE SET NULL,
    invite_url       text        CHECK (invite_url ~ '^https://' AND length(invite_url) <= 2048),
    fallback_url     text        CHECK (fallback_url ~ '^https?://' AND length(fallback_url) <= 2048),
    refresh_interval interval    CHECK (refresh_interval >= interval '1 minute'),
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),
    CHECK (
        (provider = 'discord' AND external_id ~ '^[0-9]{17,20}$') OR
        (provider = 'kook' AND external_id ~ '^[0-9]{1,20}$') OR
        (provider = 'static' AND external_id IS NULL)
    )
);
CREATE INDEX communities_page_idx ON communities (page_id);

-- One current QR code per community (v1). Replacing inserts a new row (new
-- id) and deletes the old one, so the old /media/q/{id} turns into 410.
CREATE TABLE community_qr_codes (
    id           uuid        PRIMARY KEY DEFAULT uuidv7(),
    community_id uuid        NOT NULL UNIQUE REFERENCES communities (id) ON DELETE CASCADE,
    media_key    text        NOT NULL REFERENCES media (key) ON DELETE RESTRICT,
    note         jsonb       NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(note) = 'object'),
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX community_qr_codes_media_idx ON community_qr_codes (media_key);

-- Latest provider result per community. data is the provider.Snapshot JSON
-- without member lists (users live in memory only).
CREATE TABLE provider_snapshots (
    community_id  uuid        PRIMARY KEY REFERENCES communities (id) ON DELETE CASCADE,
    data          jsonb       NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(data) = 'object' AND NOT data ? 'users'),
    state         text        NOT NULL DEFAULT 'pending'
        CHECK (state IN ('pending', 'live', 'stale', 'degraded', 'static', 'qr-only', 'unavailable')),
    err_code      text        CHECK (err_code ~ '^[a-z0-9_]{1,64}$'),
    fetched_at    timestamptz,
    last_ok_at    timestamptz,
    next_fetch_at timestamptz NOT NULL DEFAULT now(),
    fail_count    integer     NOT NULL DEFAULT 0 CHECK (fail_count >= 0)
);
CREATE INDEX provider_snapshots_next_fetch_idx ON provider_snapshots (next_fetch_at);

-- Links (link blocks and social icon rows). Followed through /go/{slug}
-- unless rel_me is set (rel="me" needs the direct profile URL).
CREATE TABLE links (
    id         uuid        PRIMARY KEY DEFAULT uuidv7(),
    page_id    smallint    NOT NULL DEFAULT 1 REFERENCES pages (id) ON DELETE CASCADE,
    slug       text        NOT NULL UNIQUE CHECK (slug ~ '^[a-z0-9][a-z0-9-]{0,63}$'),
    kind       text        NOT NULL CHECK (kind IN ('link', 'social')),
    label      jsonb       NOT NULL CHECK (jsonb_typeof(label) = 'object' AND label <> '{}'::jsonb),
    url        text        NOT NULL CHECK (url ~ '^(https?://|mailto:)' AND length(url) <= 2048),
    icon       text        CHECK (icon ~ '^(si|builtin):[a-z0-9-]{1,64}$'),
    icon_key   text        REFERENCES media (key) ON DELETE SET NULL,
    rel_me     boolean     NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (icon IS NULL OR icon_key IS NULL)
);
CREATE INDEX links_page_idx ON links (page_id);

-- Page blocks in display order. community/link blocks reference their
-- target with a real foreign key (the doc's polymorphic ref_id is split so
-- deleting a community or link removes its blocks).
--   heading:    data = {"text": LocalizedText, "showCount": bool}
--   text:       data = {"markdown": LocalizedText}
--   social_row: data = {"linkIds": [uuid, ...]}
CREATE TABLE page_blocks (
    id           uuid        PRIMARY KEY DEFAULT uuidv7(),
    page_id      smallint    NOT NULL DEFAULT 1 REFERENCES pages (id) ON DELETE CASCADE,
    kind         text        NOT NULL CHECK (kind IN ('community', 'link', 'heading', 'text', 'social_row')),
    community_id uuid        REFERENCES communities (id) ON DELETE CASCADE,
    link_id      uuid        REFERENCES links (id) ON DELETE CASCADE,
    data         jsonb       NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(data) = 'object'),
    sort_order   integer     NOT NULL,
    visible      boolean     NOT NULL DEFAULT true,
    visible_from timestamptz,
    visible_to   timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    CHECK ((kind = 'community') = (community_id IS NOT NULL)),
    CHECK ((kind = 'link') = (link_id IS NOT NULL)),
    CHECK (visible_from IS NULL OR visible_to IS NULL OR visible_from < visible_to)
);
CREATE INDEX page_blocks_page_order_idx ON page_blocks (page_id, sort_order, id);
CREATE INDEX page_blocks_community_idx ON page_blocks (community_id) WHERE community_id IS NOT NULL;
CREATE INDEX page_blocks_link_idx ON page_blocks (link_id) WHERE link_id IS NOT NULL;

-- /go/{slug} resolves communities and links from one namespace: a slug may
-- not be used by both tables. The advisory lock serializes concurrent
-- writers so the check cannot race.
-- +goose StatementBegin
CREATE FUNCTION lp_check_go_slug() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    PERFORM pg_advisory_xact_lock(hashtext('linkspage.go_slug'));
    IF TG_TABLE_NAME = 'communities' THEN
        IF EXISTS (SELECT 1 FROM links WHERE slug = NEW.slug) THEN
            RAISE EXCEPTION 'slug % is already used by a link', NEW.slug USING ERRCODE = 'unique_violation';
        END IF;
    ELSIF EXISTS (SELECT 1 FROM communities WHERE slug = NEW.slug) THEN
        RAISE EXCEPTION 'slug % is already used by a community', NEW.slug USING ERRCODE = 'unique_violation';
    END IF;
    RETURN NEW;
END
$$;
-- +goose StatementEnd

CREATE TRIGGER communities_go_slug BEFORE INSERT OR UPDATE OF slug ON communities
    FOR EACH ROW EXECUTE FUNCTION lp_check_go_slug();
CREATE TRIGGER links_go_slug BEFORE INSERT OR UPDATE OF slug ON links
    FOR EACH ROW EXECUTE FUNCTION lp_check_go_slug();

-- +goose Down
DROP TABLE page_blocks;
DROP TABLE links;
DROP TABLE provider_snapshots;
DROP TABLE community_qr_codes;
DROP TABLE communities;
DROP TABLE custom_platforms;
DROP TABLE media_proxy;
DROP TABLE media;
DROP FUNCTION lp_check_go_slug();
