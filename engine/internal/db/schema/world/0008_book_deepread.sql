-- Глубокий разбор первой реплики главы (T-07-03, CB-02): один дешёвый вызов
-- модели на главу, результат хранится целиком для аудита. chapter_id UNIQUE —
-- повторная реплика той же главы не создаёт второй разбор.
CREATE TABLE deep_reads (
    id INTEGER PRIMARY KEY,
    chapter_id INTEGER NOT NULL UNIQUE,
    intent_json TEXT NOT NULL,
    plan_json TEXT NOT NULL,
    foreboding_json TEXT NOT NULL,
    open_thread_json TEXT NOT NULL,
    citations_json TEXT NOT NULL,
    raw_json TEXT NOT NULL,
    created_at TEXT NOT NULL
);
