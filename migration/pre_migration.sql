-- ============================================================================
-- Pre-migration script
-- ============================================================================
-- Function: Remove characters not supported by PostgreSQL and clean up duplicate tags
--
-- Execution (Local):
--   mysql -u user -p e_hentai_db < pre_migration.sql
--
-- Execution (Docker):
--   docker exec -i mysql_container mysql -u user -ppassword e_hentai_db < pre_migration.sql
-- ============================================================================

BEGIN;

-- Remove NULL characters from uploader field
UPDATE gallery
SET uploader = REPLACE(uploader, CHAR(0), '')
WHERE uploader LIKE '%\0%';

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

UPDATE gid_tid gt
JOIN tmp_duplicate_map map ON gt.tid = map.dup_id
SET gt.tid = map.keep_id;

DELETE t
FROM tag t
JOIN tmp_duplicate_map map ON t.id = map.dup_id;

COMMIT;