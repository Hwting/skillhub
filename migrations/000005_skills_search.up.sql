ALTER TABLE skills
  ADD COLUMN search_vector tsvector
  GENERATED ALWAYS AS (to_tsvector('simple', name)) STORED;
CREATE INDEX skills_search_idx ON skills USING GIN (search_vector);
