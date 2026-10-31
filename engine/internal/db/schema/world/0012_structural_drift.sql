-- Structural Drift: счётчики использования семей (T-10-10, SCR-§3.3, §6.2).
-- В world(name).db ведутся family_usage_chapter[family] и
-- family_usage_book[family]; чем выше счётчик семьи в текущей главе — тем
-- сильнее штраф при выборе. Chapter-счётчики сбрасываются в OpenChapter
-- (SCR-§6.2), book-счётчики не сбрасываются. Пишет таблицу writer `core`.
CREATE TABLE family_usage (
    scope TEXT NOT NULL CHECK (scope IN ('chapter','book')),
    family TEXT NOT NULL,
    count INTEGER NOT NULL DEFAULT 0,
    updated_at TEXT NOT NULL,
    PRIMARY KEY (scope, family)
);
