-- ============================================================================
-- Pre-migration script
-- ============================================================================
-- Function: Remove characters not supported by PostgreSQL and clean up duplicate tags
--
-- Execution (Local):
--   sqlite3 e-hentai.db < pre_migration.sql
--
-- Execution (Docker):
--   docker run --rm -i -v "$PWD":/work -w /work keinos/sqlite3 sqlite3 e-hentai.db < pre_migration.sql
-- ============================================================================

BEGIN;

-- Remove NULL characters from uploader field
UPDATE gallery
SET uploader = REPLACE(uploader, CHAR(0), '')
WHERE INSTR(uploader, CHAR(0)) > 0;

-- Clean up duplicate tags
CREATE TEMPORARY TABLE tmp_duplicate_tags AS
SELECT
    name,
    MIN(id) AS keep_id
FROM tag
GROUP BY name
HAVING COUNT(*) > 1;

CREATE TEMPORARY TABLE tmp_duplicate_map AS
SELECT
    t.id   AS dup_id,
    tmp.keep_id
FROM tag t
JOIN tmp_duplicate_tags tmp ON t.name = tmp.name
WHERE t.id <> tmp.keep_id;

UPDATE gid_tid
SET tid = (
    SELECT map.keep_id
    FROM tmp_duplicate_map map
    WHERE map.dup_id = gid_tid.tid
)
WHERE tid IN (
    SELECT dup_id
    FROM tmp_duplicate_map
);

DELETE FROM gid_tid
WHERE rowid NOT IN (
    SELECT MIN(rowid)
    FROM gid_tid
    GROUP BY gid, tid
);

DELETE FROM tag
WHERE id IN (
    SELECT dup_id
    FROM tmp_duplicate_map
);

COMMIT;
