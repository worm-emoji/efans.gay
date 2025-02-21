-- Migration: Add x_post_id column and migrate post_to_x data
BEGIN;

-- Add new x_post_id column
ALTER TABLE posts ADD COLUMN x_post_id text;

-- Migrate data: set x_post_id to '0' where post_to_x was true
UPDATE posts SET x_post_id = '0' WHERE post_to_x = true;

-- Drop the old post_to_x column
ALTER TABLE posts DROP COLUMN post_to_x;

COMMIT; 