-- Add "failed" to the game_status enum.
-- PostgreSQL 12+ permits ALTER TYPE ADD VALUE inside a transaction as long as the new
-- value is not referenced in the same transaction. This migration only adds the value;
-- it does not INSERT or UPDATE any row to 'failed', so running under golang-migrate's
-- default transactional wrapping is safe. Both dev and test DBs run postgres:16-alpine.
ALTER TYPE game_status ADD VALUE IF NOT EXISTS 'failed';

-- Error message populated when a session transitions to "failed".
ALTER TABLE game_sessions ADD COLUMN error_message TEXT;

-- Current round tracker. Starts at 1; advances per §9 of the spec.
-- Single-player (Plan 4): advanced inline in the answer handler.
-- Multiplayer (Plan 5): advanced by the barrier service — same column, different trigger.
ALTER TABLE game_sessions ADD COLUMN current_round INT NOT NULL DEFAULT 1;
