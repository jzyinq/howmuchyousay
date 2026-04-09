CREATE TYPE game_mode AS ENUM ('comparison', 'guess');
CREATE TYPE game_status AS ENUM ('crawling', 'ready', 'lobby', 'in_progress', 'finished');

CREATE TABLE game_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    room_code VARCHAR(6),
    host_nick VARCHAR(50) NOT NULL,
    shop_id UUID NOT NULL REFERENCES shops(id),
    game_mode game_mode NOT NULL,
    rounds_total INT NOT NULL DEFAULT 10,
    status game_status NOT NULL DEFAULT 'crawling',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_game_sessions_room_code ON game_sessions(room_code) WHERE room_code IS NOT NULL;
