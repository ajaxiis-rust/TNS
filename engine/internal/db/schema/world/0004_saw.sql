-- Пила и арка сцены (T-03-06, LT-09): переменная нагнетания Пилы по нити
-- отношения и отдельный сигнал арки сцены, который обнуляется на границе.
-- Обе таблицы пишет writer `core`.

CREATE TABLE saw_state (
    character_id INTEGER NOT NULL,
    thread_id INTEGER NOT NULL,
    value REAL NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE UNIQUE INDEX saw_state_character_thread_idx
    ON saw_state (character_id, thread_id);

CREATE TABLE scene_arc (
    scene_id INTEGER NOT NULL,
    value REAL NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE UNIQUE INDEX scene_arc_scene_id_idx
    ON scene_arc (scene_id);
