-- Повтор оседания одной сцены не создаёт вторую запись (W-03.4, T-04-10):
-- UNIQUE по ссылке на исходную сцену. Дешёвый повтор конвейер поглощает
-- проверкой существующей записи до вызова модели и эмбеддера.
CREATE UNIQUE INDEX sediment_scene_source_scene_id_idx
    ON sediment_scene (source_scene_id);
