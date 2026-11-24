-- Теги записей банка ковки книги (T-05-07). Ковка чанка идёт до разбора книги,
-- поэтому до этого исправления bank_pool.tags оставался пустым, а выбор банка
-- требует пересечения тегов сцены с тегами записи и без него всегда даёт
-- ErrNoRecord. Миграция данных заполняет пустые теги по уже разобранным чанкам:
-- origin forge-book:<book_id>:<ord> → book_chunk.ord → ключи book_chunk_tag
-- (полоса брака не тег). Записи без разбора и не книги не трогаются.
-- Идемпотентность: UPDATE отбирает только строки с пустыми tags, повторный
-- прогон меняет ноль строк.
UPDATE bank_pool
SET tags = COALESCE((
    SELECT group_concat(key, ',')
    FROM (
        SELECT DISTINCT t.key AS key
        FROM book_chunk_tag t
        JOIN book_chunk c ON c.id = t.chunk_id
        WHERE c.book_id = bank_pool.book_id
          AND c.ord = CAST(substr(
              bank_pool.origin,
              length('forge-book:' || CAST(bank_pool.book_id AS TEXT) || ':') + 1
          ) AS INTEGER)
          AND t.band != 'reject'
        ORDER BY t.key
    )
), '')
WHERE tags = ''
  AND book_id IS NOT NULL
  AND instr(origin, 'forge-book:' || CAST(book_id AS TEXT) || ':') = 1;
