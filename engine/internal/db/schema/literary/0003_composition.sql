-- Композиция драмы в банке (SCR-§6.1, T-10-08). Аддитивно: дефолты колонок —
-- деградация старых записей (SCR-§4): до ковки композицией они живут
-- family-only (family_primary='', inversion_flag=0).
--
-- CHECK списка 14 семей в DDL намеренно нет: реестр семей — данные
-- (base/archetypes.json), а не DDL; принадлежность семьи реестру проверяют
-- писатель (T-10-12) и читатель (T-10-11).
ALTER TABLE bank_pool ADD COLUMN family_primary TEXT NOT NULL DEFAULT '';
ALTER TABLE bank_pool ADD COLUMN pattern_secondary TEXT;
ALTER TABLE bank_pool ADD COLUMN technique TEXT;
ALTER TABLE bank_pool ADD COLUMN moral_stance TEXT;
ALTER TABLE bank_pool ADD COLUMN inversion_flag INTEGER NOT NULL DEFAULT 0 CHECK (inversion_flag IN (0,1));
-- usage_count инкрементирует контур forge (ковка/переплавка/подъём Гефеста);
-- счётчики использования в живом ходе живут в оперативной базе world(name).db
-- (T-10-10) — один писатель на таблицу.
ALTER TABLE bank_pool ADD COLUMN usage_count INTEGER NOT NULL DEFAULT 0;
