CREATE TABLE workshop_queue (
    id INTEGER PRIMARY KEY,
    kind TEXT NOT NULL CHECK (kind IN ('analysis','forge_plan','complement','horoscope','pythia','edit','epoch','style','sediment','ring','wakeup')),
    payload_json TEXT NOT NULL,
    priority INTEGER NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('queued','running','done','cancelled','failed')),
    step TEXT NOT NULL CHECK (step IN ('light','heavy')),
    resumable INTEGER NOT NULL CHECK (resumable IN (0,1)),
    checkpoint_json TEXT,
    attempts INTEGER NOT NULL,
    created_at TEXT NOT NULL,
    started_at TEXT,
    finished_at TEXT
);
