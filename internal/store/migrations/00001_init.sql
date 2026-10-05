-- +goose Up
-- Pages: v1 only uses id = 1 (the default page).
CREATE TABLE pages (
    id   smallint PRIMARY KEY,
    slug text     NOT NULL UNIQUE CHECK (slug ~ '^[a-z0-9][a-z0-9-]{0,63}$')
);

-- Site settings: one row per page (a singleton in v1). "data" holds only
-- values that differ from the built-in defaults; *_by stores identity keys.
CREATE TABLE site_settings (
    page_id    smallint    PRIMARY KEY REFERENCES pages (id),
    version    bigint      NOT NULL DEFAULT 1 CHECK (version > 0),
    data       jsonb       NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(data) = 'object'),
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by text
);

-- Schema metadata: the oldest application version that can run against
-- this schema (enables rolling back one minor version).
CREATE TABLE schema_meta (
    id                         boolean PRIMARY KEY DEFAULT true CHECK (id),
    min_compatible_app_version text    NOT NULL
);

INSERT INTO pages (id, slug) VALUES (1, 'default');
INSERT INTO site_settings (page_id, version, data, updated_by) VALUES (1, 1, '{}'::jsonb, 'system');
INSERT INTO schema_meta (id, min_compatible_app_version) VALUES (true, '0.0.0');

-- +goose Down
DROP TABLE schema_meta;
DROP TABLE site_settings;
DROP TABLE pages;
