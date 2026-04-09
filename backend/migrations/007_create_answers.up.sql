CREATE TABLE answers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    round_id UUID NOT NULL REFERENCES rounds(id),
    player_id UUID NOT NULL REFERENCES players(id),
    answer TEXT NOT NULL,
    is_correct BOOLEAN NOT NULL,
    points_earned INT NOT NULL DEFAULT 0,
    answered_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(round_id, player_id)
);

CREATE INDEX idx_answers_round_id ON answers(round_id);
CREATE INDEX idx_answers_player_id ON answers(player_id);
