ALTER TABLE game_sessions DROP COLUMN IF EXISTS current_round;
ALTER TABLE game_sessions DROP COLUMN IF EXISTS error_message;
-- PostgreSQL does not support removing a value from an ENUM without recreating the type.
-- This down migration is best-effort for dev rollbacks.
