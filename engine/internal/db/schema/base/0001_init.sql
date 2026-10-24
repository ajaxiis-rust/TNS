CREATE TABLE narratives (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    arc_summary TEXT NOT NULL,
    lang TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE TABLE verses (
    id INTEGER PRIMARY KEY,
    narrative_id INTEGER NOT NULL REFERENCES narratives(id),
    text TEXT NOT NULL,
    lang TEXT NOT NULL,
    ord INTEGER NOT NULL
);

CREATE TABLE patterns (
    id INTEGER PRIMARY KEY,
    narrative_id INTEGER NOT NULL REFERENCES narratives(id),
    kind TEXT NOT NULL,
    text TEXT NOT NULL,
    lang TEXT NOT NULL
);

CREATE TABLE characters (
    id INTEGER PRIMARY KEY,
    narrative_id INTEGER NOT NULL REFERENCES narratives(id),
    name TEXT NOT NULL,
    traits_json TEXT NOT NULL
);
