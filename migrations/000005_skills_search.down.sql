DROP INDEX IF EXISTS skills_search_idx;
ALTER TABLE skills DROP COLUMN IF EXISTS search_vector;
