CREATE TABLE bank_pool (
    id INTEGER PRIMARY KEY,
    kind TEXT NOT NULL CHECK (kind IN ('routine','drama','fallback')),
    text TEXT NOT NULL,
    lang TEXT NOT NULL,
    tags TEXT NOT NULL,
    -- trust_level: 0 снятый, 1 непроверенный (оседание), 2 проверенный (сид и подъём Гефестом)
    trust_level INTEGER NOT NULL CHECK (trust_level IN (0,1,2)),
    origin TEXT NOT NULL,
    source_scene_id INTEGER,
    created_at TEXT NOT NULL
);

CREATE TABLE lit_template (
    id INTEGER PRIMARY KEY,
    kind TEXT NOT NULL,
    text TEXT NOT NULL,
    lang TEXT NOT NULL,
    tags TEXT NOT NULL
);

CREATE TABLE sediment_scene (
    id INTEGER PRIMARY KEY,
    source_scene_id INTEGER NOT NULL,
    cleaned_text TEXT NOT NULL,
    vector_id INTEGER,
    verdict TEXT NOT NULL,
    -- trust_level: 0 снятый, 1 непроверенный (оседание), 2 проверенный (сид и подъём Гефестом)
    trust_level INTEGER NOT NULL CHECK (trust_level IN (0,1,2)),
    created_at TEXT NOT NULL
);

CREATE TABLE horoscope_issue (
    id INTEGER PRIMARY KEY,
    scene_id INTEGER NOT NULL,
    issue_json TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE TABLE forge_log (
    id INTEGER PRIMARY KEY,
    kind TEXT NOT NULL,
    task TEXT NOT NULL,
    status TEXT NOT NULL,
    started_at TEXT NOT NULL,
    finished_at TEXT,
    log_path TEXT,
    package_version TEXT
);

CREATE TABLE artifact (
    id INTEGER PRIMARY KEY,
    kind TEXT NOT NULL,
    path TEXT NOT NULL,
    checksum TEXT NOT NULL
);
