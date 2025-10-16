-- ============================================================================
-- Post-migration processing script for pgloader
-- ============================================================================
-- Function: Convert raw data migrated by pgloader into an optimized PostgreSQL structure
--
-- Prerequisite: MySQL data has been imported into PostgreSQL via pgloader
--
-- Execution (Local):
--   psql -U user -d ehentai_db -f post_migration.sql
--
-- Execution (Docker):
--   docker exec -e PGPASSWORD=password -i postgres_container psql -U user -d ehentai_db < post_migration.sql
-- ============================================================================

BEGIN;

-- Set search path (new tables are created in public schema, raw data is read from e_hentai_db schema)
SET search_path TO public, e_hentai_db;

-- ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
-- Step 1: Enable required extensions
-- ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

CREATE EXTENSION IF NOT EXISTS pg_trgm;        -- Trigram fuzzy search
CREATE EXTENSION IF NOT EXISTS btree_gin;      -- B-tree types with GIN index support

-- ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
-- Step 2: Create the optimized target table structure
-- ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

CREATE TABLE gallery (
    gid             INTEGER PRIMARY KEY,
    token           CHAR(10) NOT NULL,
    archiver_key    VARCHAR(60) NOT NULL DEFAULT '',
    title           VARCHAR(512) NOT NULL,
    title_jpn       VARCHAR(512) NOT NULL DEFAULT '',
    category        VARCHAR(15) NOT NULL,
    thumb           VARCHAR(150) NOT NULL DEFAULT '',
    uploader        VARCHAR(50) DEFAULT NULL,
    posted          TIMESTAMPTZ NOT NULL,
    filecount       INTEGER NOT NULL,
    filesize        BIGINT NOT NULL,
    expunged        BOOLEAN NOT NULL DEFAULT FALSE,
    removed         BOOLEAN NOT NULL DEFAULT FALSE,
    replaced        BOOLEAN NOT NULL DEFAULT FALSE,
    rating          NUMERIC(4,2) NOT NULL DEFAULT 0.00,
    torrentcount    INTEGER NOT NULL DEFAULT 0,
    root_gid        INTEGER DEFAULT NULL,
    bytorrent       BOOLEAN NOT NULL DEFAULT FALSE,
    tags            JSONB NOT NULL DEFAULT '[]'::jsonb,
    title_tsv       tsvector GENERATED ALWAYS AS (
                        setweight(to_tsvector('simple', coalesce(title, '')), 'A') ||
                        setweight(to_tsvector('simple', coalesce(title_jpn, '')), 'B')
                    ) STORED
);

CREATE TABLE tag (
    id              SERIAL PRIMARY KEY,
    name            VARCHAR(200) NOT NULL UNIQUE
);

CREATE TABLE torrent (
    id              INTEGER NOT NULL,
    gid             INTEGER NOT NULL,
    name            VARCHAR(300) NOT NULL,
    hash            CHAR(40) DEFAULT NULL,
    addedstr        VARCHAR(20) DEFAULT NULL,
    fsizestr        VARCHAR(15) DEFAULT NULL,
    uploader        VARCHAR(50) NOT NULL,
    expunged        BOOLEAN NOT NULL DEFAULT FALSE,
    PRIMARY KEY (id, gid)
);

-- ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
-- Step 3: Convert and import gallery data
-- ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

INSERT INTO gallery (
    gid, token, archiver_key, title, title_jpn, category, thumb, uploader,
    posted, filecount, filesize, expunged, removed, replaced, rating,
    torrentcount, root_gid, bytorrent, tags
)
SELECT
    g.gid,
    g.token,
    COALESCE(g.archiver_key, ''),
    g.title,
    COALESCE(g.title_jpn, ''),
    g.category,
    COALESCE(g.thumb, ''),
    g.uploader,
    to_timestamp(g.posted) AT TIME ZONE 'UTC',
    g.filecount,
    g.filesize,
    g.expunged != 0,
    g.removed != 0,
    g.replaced != 0,
    CAST(g.rating AS NUMERIC(4,2)),
    g.torrentcount,
    g.root_gid,
    g.bytorrent != 0,
    COALESCE(
        (SELECT jsonb_agg(t.name ORDER BY t.name)
         FROM e_hentai_db.gid_tid gt
         INNER JOIN e_hentai_db.tag t ON gt.tid = t.id
         WHERE gt.gid = g.gid),
        '[]'::jsonb
    ) AS tags
FROM e_hentai_db.gallery g
ORDER BY g.gid;

-- ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
-- Step 4: Import tag data
-- ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

INSERT INTO tag (id, name)
SELECT id, name
FROM e_hentai_db.tag
ORDER BY id;

-- Update sequence
SELECT setval('public.tag_id_seq', (SELECT MAX(id) FROM public.tag));

-- ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
-- Step 5: Import torrent data
-- ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

INSERT INTO torrent (id, gid, name, hash, addedstr, fsizestr, uploader, expunged)
SELECT
    id, gid, name, hash,
    addedstr, fsizestr, uploader, expunged != 0  -- smallint -> boolean
FROM e_hentai_db.torrent
ORDER BY id, gid;

-- ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
-- Step 6: Create all indexes
-- ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

-- Gallery table indexes
CREATE INDEX idx_gallery_title_tsv ON gallery USING GIN (title_tsv);

CREATE INDEX idx_gallery_title_trgm ON gallery USING GIN (title gin_trgm_ops);
CREATE INDEX idx_gallery_title_jpn_trgm ON gallery USING GIN (title_jpn gin_trgm_ops);
CREATE INDEX idx_gallery_uploader_trgm ON gallery USING GIN (uploader gin_trgm_ops);

CREATE INDEX idx_gallery_tags ON gallery USING GIN (tags);

CREATE INDEX idx_gallery_token ON gallery (token);
CREATE INDEX idx_gallery_category ON gallery (category);
CREATE INDEX idx_gallery_uploader ON gallery (uploader) WHERE uploader IS NOT NULL;
CREATE INDEX idx_gallery_root_gid ON gallery (root_gid) WHERE root_gid IS NOT NULL;
CREATE INDEX idx_gallery_rating ON gallery (rating);

CREATE INDEX idx_gallery_category_exp_posted ON gallery (category, expunged, posted DESC);
CREATE INDEX idx_gallery_uploader_exp_posted ON gallery (uploader, expunged, posted DESC) WHERE uploader IS NOT NULL;
CREATE INDEX idx_gallery_exp_removed_replaced ON gallery (expunged, removed, replaced);

CREATE INDEX idx_gallery_posted_brin ON gallery USING BRIN (posted) WITH (pages_per_range = 128);

-- Tag table indexes
CREATE INDEX idx_tag_name_trgm ON tag USING GIN (name gin_trgm_ops);

-- Torrent table indexes
CREATE INDEX idx_torrent_gid ON torrent (gid);
CREATE INDEX idx_torrent_hash ON torrent (hash) WHERE hash IS NOT NULL;

-- ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
-- Step 7: Create materialized views
-- ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

CREATE MATERIALIZED VIEW gallery_stats_mv AS
SELECT
    'total_active' AS stat_key,
    COUNT(*) AS stat_value,
    NOW() AS updated_at
FROM gallery
WHERE expunged = FALSE

UNION ALL

SELECT
    'total_removed' AS stat_key,
    COUNT(*) AS stat_value,
    NOW() AS updated_at
FROM gallery
WHERE removed = TRUE

UNION ALL

SELECT
    'total_replaced' AS stat_key,
    COUNT(*) AS stat_value,
    NOW() AS updated_at
FROM gallery
WHERE replaced = TRUE

UNION ALL

SELECT
    'total_expunged' AS stat_key,
    COUNT(*) AS stat_value,
    NOW() AS updated_at
FROM gallery
WHERE expunged = TRUE

UNION ALL

SELECT
    CONCAT('category_', LOWER(category)) AS stat_key,
    COUNT(*) AS stat_value,
    NOW() AS updated_at
FROM gallery
WHERE expunged = FALSE
GROUP BY category;

CREATE UNIQUE INDEX idx_gallery_stats_mv_key ON gallery_stats_mv (stat_key);

CREATE MATERIALIZED VIEW uploader_stats_mv AS
SELECT
    uploader,
    COUNT(*) AS gallery_count,
    SUM(filecount) AS total_pages,
    SUM(filesize) AS total_size,
    AVG(rating) AS avg_rating,
    NOW() AS updated_at
FROM gallery
WHERE expunged = FALSE
  AND removed = FALSE
  AND uploader IS NOT NULL
GROUP BY uploader
HAVING COUNT(*) >= 5
ORDER BY gallery_count DESC;

CREATE UNIQUE INDEX idx_uploader_stats_mv_uploader ON uploader_stats_mv (uploader);
CREATE INDEX idx_uploader_stats_mv_count ON uploader_stats_mv (gallery_count DESC);

CREATE MATERIALIZED VIEW tag_stats_mv AS
SELECT
    tag_name,
    gallery_count,
    NOW() AS updated_at
FROM (
    SELECT
        jsonb_array_elements_text(tags) AS tag_name,
        COUNT(*) AS gallery_count
    FROM gallery
    WHERE expunged = FALSE
      AND removed = FALSE
    GROUP BY jsonb_array_elements_text(tags)
) AS tag_counts
WHERE gallery_count >= 3
ORDER BY gallery_count DESC;

CREATE UNIQUE INDEX idx_tag_stats_mv_tag ON tag_stats_mv (tag_name);
CREATE INDEX idx_tag_stats_mv_count ON tag_stats_mv (gallery_count DESC);
CREATE INDEX idx_tag_stats_mv_tag_trgm ON tag_stats_mv USING GIN (tag_name gin_trgm_ops);

-- ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
-- Step 8: Create helper functions
-- ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

CREATE OR REPLACE FUNCTION refresh_all_stats(concurrent_mode BOOLEAN DEFAULT TRUE)
RETURNS TEXT AS $$
DECLARE
    start_time TIMESTAMP;
    result_text TEXT;
BEGIN
    start_time := clock_timestamp();

    IF concurrent_mode THEN
        REFRESH MATERIALIZED VIEW CONCURRENTLY gallery_stats_mv;
        REFRESH MATERIALIZED VIEW CONCURRENTLY uploader_stats_mv;
        REFRESH MATERIALIZED VIEW CONCURRENTLY tag_stats_mv;
        result_text := 'All stats refreshed concurrently';
    ELSE
        REFRESH MATERIALIZED VIEW gallery_stats_mv;
        REFRESH MATERIALIZED VIEW uploader_stats_mv;
        REFRESH MATERIALIZED VIEW tag_stats_mv;
        result_text := 'All stats refreshed';
    END IF;

    RETURN result_text || ' in ' ||
           EXTRACT(EPOCH FROM (clock_timestamp() - start_time))::TEXT || ' seconds';
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION search_tags_by_prefix(tag_prefix TEXT, result_limit INTEGER DEFAULT 20)
RETURNS TABLE (
    tag_name TEXT,
    usage_count BIGINT
) AS $$
BEGIN
    RETURN QUERY
    SELECT
        ts.tag_name::TEXT,
        ts.gallery_count
    FROM tag_stats_mv ts
    WHERE ts.tag_name ILIKE (tag_prefix || '%')
    ORDER BY ts.gallery_count DESC
    LIMIT result_limit;
END;
$$ LANGUAGE plpgsql STABLE;

CREATE OR REPLACE FUNCTION search_gallery_title(
    search_text TEXT,
    result_limit INTEGER DEFAULT NULL
)
RETURNS TABLE (
    gid INTEGER,
    token CHAR(10),
    archiver_key VARCHAR(60),
    title VARCHAR(512),
    title_jpn VARCHAR(512),
    category VARCHAR(15),
    thumb VARCHAR(150),
    uploader VARCHAR(50),
    posted TIMESTAMPTZ,
    filecount INTEGER,
    filesize BIGINT,
    expunged BOOLEAN,
    removed BOOLEAN,
    replaced BOOLEAN,
    rating NUMERIC(4,2),
    torrentcount INTEGER,
    root_gid INTEGER,
    bytorrent BOOLEAN,
    tags JSONB,
    relevance REAL
) AS $$
DECLARE
    search_words TEXT[];
    word TEXT;
    title_conditions TEXT := '';
    title_jpn_conditions TEXT := '';
    title_score_conditions TEXT := '';
    title_jpn_score_conditions TEXT := '';
    has_fulltext BOOLEAN;
    query TEXT;
BEGIN
    -- Split search text into words by space
    search_words := string_to_array(trim(search_text), ' ');
    search_words := array_remove(search_words, '');

    -- Pre-check if full-text search is possible
    has_fulltext := websearch_to_tsquery('simple', search_text)::text != '';

    -- Dynamically build ILIKE conditions (to avoid subqueries)
    IF array_length(search_words, 1) > 0 THEN
        -- Build WHERE conditions (all words must match)
        FOREACH word IN ARRAY search_words LOOP
            IF title_conditions != '' THEN
                title_conditions := title_conditions || ' AND ';
                title_jpn_conditions := title_jpn_conditions || ' AND ';
            END IF;
            title_conditions := title_conditions || format('g.title ILIKE %L', '%' || word || '%');
            title_jpn_conditions := title_jpn_conditions || format('g.title_jpn ILIKE %L', '%' || word || '%');
        END LOOP;

        -- Build scoring conditions
        title_score_conditions := format('CASE WHEN (%s) THEN 0.4 ELSE 0 END', title_conditions);
        title_jpn_score_conditions := format('CASE WHEN (%s) THEN 0.4 ELSE 0 END', title_jpn_conditions);
    ELSE
        -- If there are no search words, set default conditions
        title_conditions := 'FALSE';
        title_jpn_conditions := 'FALSE';
        title_score_conditions := '0';
        title_jpn_score_conditions := '0';
    END IF;

    -- Build dynamic query
    IF result_limit IS NULL THEN
        query := format($sql$
            SELECT
                g.gid,
                g.token,
                g.archiver_key,
                g.title,
                g.title_jpn,
                g.category,
                g.thumb,
                g.uploader,
                g.posted,
                g.filecount,
                g.filesize,
                g.expunged,
                g.removed,
                g.replaced,
                g.rating,
                g.torrentcount,
                g.root_gid,
                g.bytorrent,
                g.tags,
                (
                    CASE
                        WHEN %L AND g.title_tsv @@ websearch_to_tsquery('simple', %L)
                        THEN ts_rank(g.title_tsv, websearch_to_tsquery('simple', %L)) * 2
                        ELSE 0
                    END
                    + %s
                    + %s
                )::REAL AS relevance
            FROM gallery g
            WHERE
                g.expunged = FALSE
                AND (
                    (%L AND g.title_tsv @@ websearch_to_tsquery('simple', %L))
                    OR (%s)
                    OR (%s)
                )
            ORDER BY relevance DESC, g.posted DESC
        $sql$,
            has_fulltext, search_text, search_text,
            title_score_conditions, title_jpn_score_conditions,
            has_fulltext, search_text,
            title_conditions, title_jpn_conditions
        );
    ELSE
        query := format($sql$
            SELECT
                g.gid,
                g.token,
                g.archiver_key,
                g.title,
                g.title_jpn,
                g.category,
                g.thumb,
                g.uploader,
                g.posted,
                g.filecount,
                g.filesize,
                g.expunged,
                g.removed,
                g.replaced,
                g.rating,
                g.torrentcount,
                g.root_gid,
                g.bytorrent,
                g.tags,
                (
                    CASE
                        WHEN %L AND g.title_tsv @@ websearch_to_tsquery('simple', %L)
                        THEN ts_rank(g.title_tsv, websearch_to_tsquery('simple', %L)) * 2
                        ELSE 0
                    END
                    + %s
                    + %s
                )::REAL AS relevance
            FROM gallery g
            WHERE
                g.expunged = FALSE
                AND (
                    (%L AND g.title_tsv @@ websearch_to_tsquery('simple', %L))
                    OR (%s)
                    OR (%s)
                )
            ORDER BY relevance DESC, g.posted DESC
            LIMIT %L
        $sql$,
            has_fulltext, search_text, search_text,
            title_score_conditions, title_jpn_score_conditions,
            has_fulltext, search_text,
            title_conditions, title_jpn_conditions,
            result_limit
        );
    END IF;

    RETURN QUERY EXECUTE query;
END;
$$ LANGUAGE plpgsql STABLE;

CREATE OR REPLACE FUNCTION search_gallery_fuzzy(
    search_text TEXT,
    result_limit INTEGER DEFAULT NULL,
    min_similarity REAL DEFAULT 0.05
)
RETURNS TABLE (
    gid INTEGER,
    token CHAR(10),
    archiver_key VARCHAR(60),
    title VARCHAR(512),
    title_jpn VARCHAR(512),
    category VARCHAR(15),
    thumb VARCHAR(150),
    uploader VARCHAR(50),
    posted TIMESTAMPTZ,
    filecount INTEGER,
    filesize BIGINT,
    expunged BOOLEAN,
    removed BOOLEAN,
    replaced BOOLEAN,
    rating NUMERIC(4,2),
    torrentcount INTEGER,
    root_gid INTEGER,
    bytorrent BOOLEAN,
    tags JSONB,
    similarity_score REAL
) AS $$
DECLARE
    ilike_count INTEGER;
    limit_clause TEXT;
BEGIN
    -- Build LIMIT clause
    IF result_limit IS NULL THEN
        limit_clause := '';
    ELSE
        limit_clause := 'LIMIT ' || result_limit;
    END IF;

    -- First, try a fast ILIKE search
    RETURN QUERY EXECUTE format($sql$
        SELECT
            g.gid,
            g.token,
            g.archiver_key,
            g.title,
            g.title_jpn,
            g.category,
            g.thumb,
            g.uploader,
            g.posted,
            g.rating,
            g.torrentcount,
            g.root_gid,
            g.bytorrent,
            g.tags,
            g.filecount,
            GREATEST(
                similarity(g.title, %L),
                similarity(g.title_jpn, %L)
            )::REAL AS similarity_score
        FROM gallery g
        WHERE
            g.expunged = FALSE
            AND (
                g.title ILIKE %L
                OR g.title_jpn ILIKE %L
            )
        ORDER BY similarity_score DESC, g.posted DESC
        %s
    $sql$, search_text, search_text,
           '%' || search_text || '%', '%' || search_text || '%',
           limit_clause);

    -- Check the number of returned rows
    GET DIAGNOSTICS ilike_count = ROW_COUNT;

    -- If ILIKE results are insufficient, then use similarity
    IF result_limit IS NULL OR ilike_count < result_limit THEN
        IF result_limit IS NULL THEN
            limit_clause := '';
        ELSE
            limit_clause := 'LIMIT ' || (result_limit - ilike_count);
        END IF;

        RETURN QUERY EXECUTE format($sql$
            SELECT
                g.gid,
                g.token,
                g.archiver_key,
                g.title,
                g.title_jpn,
                g.category,
                g.thumb,
                g.uploader,
                g.posted,
                g.rating,
                g.torrentcount,
                g.root_gid,
                g.bytorrent,
                g.tags,
                g.filecount,
                GREATEST(
                    similarity(g.title, %L),
                    similarity(g.title_jpn, %L)
                )::REAL AS similarity_score
            FROM gallery g
            WHERE
                g.expunged = FALSE
                AND g.title NOT ILIKE %L
                AND g.title_jpn NOT ILIKE %L
                AND (
                    similarity(g.title, %L) > %L
                    OR similarity(g.title_jpn, %L) > %L
                )
            ORDER BY similarity_score DESC, g.posted DESC
            %s
        $sql$, search_text, search_text,
               '%' || search_text || '%', '%' || search_text || '%',
               search_text, min_similarity, search_text, min_similarity,
               limit_clause);
    END IF;
END;
$$ LANGUAGE plpgsql STABLE;

-- ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
-- Step 9: Add table comments
-- ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

COMMENT ON TABLE gallery IS 'E-Hentai main gallery table';
COMMENT ON COLUMN gallery.tags IS 'Tags stored as a JSONB array, e.g., ["language:chinese", "other:full color"]';
COMMENT ON COLUMN gallery.title_tsv IS 'Full-text search vector for title (auto-generated)';
COMMENT ON COLUMN gallery.posted IS 'Publication time (timestamp with time zone, UTC)';

COMMENT ON TABLE tag IS 'Normalized tag table (for autocomplete and tag lists)';
COMMENT ON TABLE torrent IS 'Torrent information table';

COMMENT ON MATERIALIZED VIEW gallery_stats_mv IS 'Gallery statistics materialized view';
COMMENT ON MATERIALIZED VIEW uploader_stats_mv IS 'Uploader statistics materialized view';
COMMENT ON MATERIALIZED VIEW tag_stats_mv IS 'Tag statistics materialized view';

COMMENT ON FUNCTION refresh_all_stats IS 'Refreshes all statistics materialized views';
COMMENT ON FUNCTION search_tags_by_prefix IS 'Search tags by prefix (for autocomplete)';
COMMENT ON FUNCTION search_gallery_title IS 'Fast title search (supports space-separated multi-word AND search, returns all results by default, millisecond response)';
COMMENT ON FUNCTION search_gallery_fuzzy IS 'Smart fuzzy search (tries fast ILIKE first, uses similarity for fault tolerance if results are insufficient, returns all results by default)';

-- ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
-- Step 10: Execute ANALYZE to collect statistics
-- ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

ANALYZE gallery;
ANALYZE tag;
ANALYZE torrent;

-- ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
-- Step 11: Drop temporary tables
-- ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

DROP TABLE IF EXISTS e_hentai_db.gallery CASCADE;
DROP TABLE IF EXISTS e_hentai_db.tag CASCADE;
DROP TABLE IF EXISTS e_hentai_db.torrent CASCADE;
DROP TABLE IF EXISTS e_hentai_db.gid_tid CASCADE;  -- Many-to-many relationship table no longer needed

-- Drop the schema created by pgloader
DROP SCHEMA IF EXISTS e_hentai_db CASCADE;

COMMIT;

-- ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
-- Some test cases
-- ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

-- 1. Test tag search (JSONB):
--    SELECT * FROM gallery WHERE tags @> '["language:chinese", "other:full color", "female:orgasm denial"]'::jsonb;
--
-- 2. Test time range query:
--    SELECT * FROM gallery WHERE posted > NOW() - INTERVAL '30 days' LIMIT 10;
--
-- 3. Test fast title search (recommended, supports multi-word search, millisecond-level):
--    -- Single word search (returns all results)
--    SELECT * FROM search_gallery_title('狡兔屋');
--
--    -- Multi-word search (space-separated, must contain all words)
--    SELECT * FROM search_gallery_title('狡兔屋 危机');
--    SELECT * FROM search_gallery_title('Cunning Hares Crisis');
--
--    -- Limit to 20 results
--    SELECT * FROM search_gallery_title('狡兔屋', 20);
--
-- 4. Test fuzzy search (fault-tolerant, supports typos):
--    -- Returns all results, default similarity 0.05
--    SELECT * FROM search_gallery_fuzzy('狡免屋');
--
--    -- Limit to 20 results, custom similarity 0.1
--    SELECT * FROM search_gallery_fuzzy('狡免屋', 20, 0.1);
--
-- 5. Test native full-text search (English only):
--    SELECT * FROM gallery
--    WHERE title_tsv @@ websearch_to_tsquery('simple', 'love story')
--    LIMIT 10;
--
-- 6. For periodic refresh of statistics views, it is recommended to install the pg_cron extension and set up a cron job:
--    SELECT cron.schedule('refresh-stats', '0 */6 * * *', $$SELECT refresh_all_stats()$$);
