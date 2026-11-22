-- Профиль игрока: психотип и привычки карты Пифии (T-17-04, W-12.2/W-12.4).
-- Нормы: психотип пишется при первом вычислении и не пересчитывается;
-- привычки пополняются по паттернам реплик. Колонки аддитивны: старые строки
-- получают пустые значения.
-- Пересборка вместо ALTER TABLE ADD COLUMN: новые миры получают обе колонки из
-- 0001_init, и повторный ALTER упал бы на «duplicate column name». Копия читает
-- только колонки, общие для обеих форм таблицы; индексов у player_atlas нет.
CREATE TABLE player_atlas_new (
    id INTEGER PRIMARY KEY,
    day INTEGER NOT NULL,
    theme TEXT NOT NULL,
    mood TEXT NOT NULL,
    interest TEXT NOT NULL,
    source TEXT NOT NULL,
    psychotype TEXT NOT NULL DEFAULT '',
    habits TEXT NOT NULL DEFAULT ''
);

INSERT INTO player_atlas_new (id, day, theme, mood, interest, source)
    SELECT id, day, theme, mood, interest, source FROM player_atlas;

DROP TABLE player_atlas;
ALTER TABLE player_atlas_new RENAME TO player_atlas;
