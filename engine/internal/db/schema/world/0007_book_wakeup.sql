-- Пробуждение главы (T-07-02, CB-01): выковывается заранее и подаётся при
-- открытии главы. chapter_id UNIQUE — повторная ковка главы не создаёт вторую
-- запись, а переводит forging → ready (upsert в book/wake.go).
CREATE TABLE wakeups (
    id INTEGER PRIMARY KEY,
    chapter_id INTEGER NOT NULL UNIQUE,
    text TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL CHECK (status IN ('forging','ready')),
    forged_at TEXT,
    delivered_at TEXT
);
