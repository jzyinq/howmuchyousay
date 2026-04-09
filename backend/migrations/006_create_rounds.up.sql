CREATE TYPE round_type AS ENUM ('comparison', 'guess');

CREATE TABLE rounds (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id UUID NOT NULL REFERENCES game_sessions(id),
    round_number INT NOT NULL,
    round_type round_type NOT NULL,
    product_a_id UUID NOT NULL REFERENCES products(id),
    product_b_id UUID REFERENCES products(id),
    correct_answer TEXT NOT NULL,
    difficulty_score INT NOT NULL DEFAULT 1,
    UNIQUE(session_id, round_number)
);

CREATE INDEX idx_rounds_session_id ON rounds(session_id);
