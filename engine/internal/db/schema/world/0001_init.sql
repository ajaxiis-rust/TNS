CREATE TABLE world_meta (
    "key" TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE TABLE characters (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    psychotype TEXT NOT NULL,
    estate TEXT NOT NULL,
    card_json TEXT NOT NULL
);

CREATE TABLE character_layers (
    character_id INTEGER NOT NULL,
    layer TEXT NOT NULL,
    payload_json TEXT NOT NULL
);

CREATE TABLE character_state (
    character_id INTEGER PRIMARY KEY,
    place_id INTEGER,
    x REAL,
    y REAL,
    mood TEXT,
    status TEXT
);

CREATE TABLE memory_topics (
    id INTEGER PRIMARY KEY,
    character_id INTEGER NOT NULL,
    theme TEXT NOT NULL,
    weight REAL NOT NULL,
    reinforced_at TEXT NOT NULL,
    vector_id INTEGER
);

CREATE TABLE sessions (
    id INTEGER PRIMARY KEY,
    started_at TEXT NOT NULL,
    ended_at TEXT,
    mode TEXT NOT NULL
);

CREATE TABLE turns (
    id INTEGER PRIMARY KEY,
    session_id INTEGER NOT NULL,
    player_text TEXT NOT NULL,
    reply_text TEXT NOT NULL,
    path TEXT,
    selector_decision TEXT,
    saw_recommendation INTEGER NOT NULL,
    trust_level INTEGER NOT NULL,
    latency_ms INTEGER NOT NULL,
    degraded INTEGER NOT NULL
);

CREATE TABLE scenes (
    id INTEGER PRIMARY KEY,
    session_id INTEGER NOT NULL,
    day INTEGER NOT NULL,
    place_id INTEGER,
    status TEXT NOT NULL,
    prose TEXT NOT NULL,
    turn_id INTEGER
);

CREATE TABLE collisions (
    id INTEGER PRIMARY KEY,
    place_id INTEGER NOT NULL,
    day INTEGER NOT NULL,
    kind TEXT NOT NULL,
    payload_json TEXT NOT NULL
);

CREATE TABLE events (
    id INTEGER PRIMARY KEY,
    day INTEGER NOT NULL,
    kind TEXT NOT NULL,
    payload_json TEXT NOT NULL,
    source TEXT NOT NULL
);

CREATE TABLE deferred_events (
    id INTEGER PRIMARY KEY,
    kind TEXT NOT NULL,
    payload_json TEXT NOT NULL,
    due_day INTEGER NOT NULL,
    status TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE TABLE places (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    kind TEXT NOT NULL,
    x REAL,
    y REAL,
    epoch_ref TEXT
);

CREATE TABLE paths (
    id INTEGER PRIMARY KEY,
    from_place_id INTEGER NOT NULL,
    to_place_id INTEGER NOT NULL,
    -- W-06.2: дни пути никогда не хранятся
    type TEXT NOT NULL CHECK (type IN ('trakt','river','sea')),
    length_km REAL,
    friction REAL
);

CREATE TABLE rings (
    id INTEGER PRIMARY KEY,
    status TEXT NOT NULL,
    progress REAL NOT NULL,
    budget_used REAL NOT NULL,
    started_at TEXT NOT NULL,
    finished_at TEXT
);

CREATE TABLE corridors (
    id INTEGER PRIMARY KEY,
    ring_id INTEGER NOT NULL,
    from_place_id INTEGER NOT NULL,
    to_place_id INTEGER NOT NULL,
    status TEXT NOT NULL
);

CREATE TABLE player_atlas (
    id INTEGER PRIMARY KEY,
    day INTEGER NOT NULL,
    theme TEXT NOT NULL,
    mood TEXT NOT NULL,
    interest TEXT NOT NULL,
    source TEXT NOT NULL
);

CREATE TABLE player_map (
    place_id INTEGER PRIMARY KEY,
    known_at TEXT NOT NULL,
    drawn INTEGER NOT NULL
);

CREATE TABLE economy (
    id INTEGER PRIMARY KEY,
    day INTEGER NOT NULL,
    resource TEXT NOT NULL,
    amount REAL NOT NULL,
    actor_id INTEGER
);

CREATE TABLE social_graph (
    id INTEGER PRIMARY KEY,
    a_character_id INTEGER NOT NULL,
    b_character_id INTEGER NOT NULL,
    relation TEXT NOT NULL,
    weight REAL NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE model_bindings (
    id INTEGER PRIMARY KEY,
    role TEXT NOT NULL,
    model_ref TEXT NOT NULL,
    provider TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE token_bindings (
    id INTEGER PRIMARY KEY,
    scene_id INTEGER,
    chapter_id INTEGER,
    token TEXT NOT NULL,
    character_id INTEGER NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE book_chapters (
    id INTEGER PRIMARY KEY,
    day INTEGER NOT NULL,
    title TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('open','closed','processing','published')),
    kind TEXT NOT NULL CHECK (kind IN ('chapter','epilogue')),
    digest TEXT
);

CREATE TABLE book_text (
    id INTEGER PRIMARY KEY,
    chapter_id INTEGER NOT NULL,
    version INTEGER NOT NULL,
    text TEXT NOT NULL,
    edited_by TEXT CHECK (edited_by IN ('player','editor') OR edited_by IS NULL),
    published INTEGER NOT NULL
);

CREATE TABLE book_cuts (
    id INTEGER PRIMARY KEY,
    chapter_id INTEGER NOT NULL,
    cut_text TEXT NOT NULL,
    reason TEXT NOT NULL,
    status TEXT NOT NULL
);
