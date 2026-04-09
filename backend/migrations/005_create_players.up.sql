CREATE TABLE players (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id UUID NOT NULL REFERENCES game_sessions(id),
    nick VARCHAR(50) NOT NULL,
    joined_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    is_host BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE INDEX idx_players_session_id ON players(session_id);
