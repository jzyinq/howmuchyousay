CREATE TYPE crawl_status AS ENUM ('pending', 'in_progress', 'completed', 'failed');

CREATE TABLE crawls (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    shop_id UUID NOT NULL REFERENCES shops(id),
    session_id UUID REFERENCES game_sessions(id),
    status crawl_status NOT NULL DEFAULT 'pending',
    products_found INT NOT NULL DEFAULT 0,
    pages_visited INT NOT NULL DEFAULT 0,
    ai_requests_count INT NOT NULL DEFAULT 0,
    error_message TEXT,
    log_file_path TEXT NOT NULL DEFAULT '',
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at TIMESTAMPTZ,
    duration_ms INT
);

ALTER TABLE game_sessions ADD COLUMN crawl_id UUID REFERENCES crawls(id);
