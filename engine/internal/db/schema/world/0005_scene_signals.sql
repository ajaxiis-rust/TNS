-- Сигналы игрока о сцене (T-04-06, W-04.1): like поднимает приоритет оседания,
-- dislike отклоняет сцену до очистки. Пишет writer `core` (точка вызова — чат,
-- T-08-15: POST /api/scene/{id}/signal); оседание только читает сигнал для отбора.

CREATE TABLE scene_signals (
    scene_id INTEGER NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('like','dislike')),
    created_at TEXT NOT NULL
);
