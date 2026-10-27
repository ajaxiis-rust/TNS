-- Эпилог и метки пиков (T-07-13, CB-13): эпилог — единственная глава чистого
-- авторства игрока; book_peaks — авторская разметка важных/поворотных моментов
-- (сцен и глав). Сцена метится по scene_id, глава — chapter_id (scene_id NULL).
-- Колонка book_chapters.kind с CHECK ('chapter','epilogue') уже создана в
-- 0001_init.sql (T-01-06) и здесь не добавляется повторно — повторный ALTER
-- упал бы на дубликате колонки.
CREATE TABLE book_peaks (
    id INTEGER PRIMARY KEY,
    chapter_id INTEGER NOT NULL,
    scene_id INTEGER,
    kind TEXT NOT NULL CHECK (kind IN ('scene','chapter')),
    note TEXT NOT NULL,
    created_at TEXT NOT NULL
);
