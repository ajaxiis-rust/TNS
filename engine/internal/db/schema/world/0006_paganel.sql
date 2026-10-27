-- Паганель: кольца, черновики мест и коридоры (T-06-05, W-09.1/W-09.3).
-- Миграция аддитивна: существующие строки не меняются, только добавляются
-- колонки. Статусы колец меняет только ядро (писатель core).

-- places/paths получают происхождение и статус записи (W-05.9): до T-06-05
-- всё выкованное считалось forge/baked; черновики Паганеля появляются в
-- T-06-08. pinned — книжная точка смешанного мира (W-07.4), 0/1.
ALTER TABLE places ADD COLUMN origin TEXT NOT NULL DEFAULT 'forge';
ALTER TABLE places ADD COLUMN status TEXT NOT NULL DEFAULT 'baked';
ALTER TABLE places ADD COLUMN pinned INTEGER NOT NULL DEFAULT 0;

ALTER TABLE paths ADD COLUMN origin TEXT NOT NULL DEFAULT 'forge';
ALTER TABLE paths ADD COLUMN status TEXT NOT NULL DEFAULT 'baked';

-- rings: CHECK по статусу нельзя добавить ALTER'ом, поэтому таблица
-- пересоздаётся; строки копируются без изменений (аддитивность).
CREATE TABLE rings_new (
    id INTEGER PRIMARY KEY,
    status TEXT NOT NULL CHECK (status IN ('queued','running','done')),
    progress REAL NOT NULL,
    budget_used REAL NOT NULL,
    started_at TEXT NOT NULL,
    finished_at TEXT,
    places_count INTEGER NOT NULL DEFAULT 0,
    paths_count INTEGER NOT NULL DEFAULT 0,
    people_count INTEGER NOT NULL DEFAULT 0
);

INSERT INTO rings_new (id, status, progress, budget_used, started_at, finished_at)
    SELECT id, status, progress, budget_used, started_at, finished_at FROM rings;

DROP TABLE rings;
ALTER TABLE rings_new RENAME TO rings;

-- corridors: опорные точки, остановки и обещанный срок коридора (T-06-09).
ALTER TABLE corridors ADD COLUMN waypoints_json TEXT;
ALTER TABLE corridors ADD COLUMN stops_json TEXT;
ALTER TABLE corridors ADD COLUMN promised_days REAL;
