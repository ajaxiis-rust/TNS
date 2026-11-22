-- DEC-6, T-17-06: в исторических записях литературной базы мира пополнитель
-- назван «Пополняется Forge»; норма — в русском тексте только «Гефест»,
-- `Forge` остаётся код-идентификатором. Писателя в текущем коде нет, запись
-- досталась от TNS35, поэтому перенос — миграция данных при открытии мира.
-- Обе формы старой записи (с обратными кавычками и без) заменяются; прочие
-- тексты не трогаются. Идемпотентность: UPDATE отбирает только строки с
-- точной фразой, повторный прогон меняет ноль строк.

UPDATE bank_pool SET
    text = REPLACE(REPLACE(text, 'Пополняется `Forge`', 'пополняется Гефестом'), 'Пополняется Forge', 'пополняется Гефестом'),
    origin = REPLACE(REPLACE(origin, 'Пополняется `Forge`', 'пополняется Гефестом'), 'Пополняется Forge', 'пополняется Гефестом')
WHERE instr(text, 'Пополняется') > 0 AND instr(text, 'Forge') > 0
   OR instr(origin, 'Пополняется') > 0 AND instr(origin, 'Forge') > 0;

UPDATE lit_template SET
    text = REPLACE(REPLACE(text, 'Пополняется `Forge`', 'пополняется Гефестом'), 'Пополняется Forge', 'пополняется Гефестом')
WHERE instr(text, 'Пополняется') > 0 AND instr(text, 'Forge') > 0;

UPDATE sediment_scene SET
    cleaned_text = REPLACE(REPLACE(cleaned_text, 'Пополняется `Forge`', 'пополняется Гефестом'), 'Пополняется Forge', 'пополняется Гефестом')
WHERE instr(cleaned_text, 'Пополняется') > 0 AND instr(cleaned_text, 'Forge') > 0;

UPDATE horoscope_issue SET
    issue_json = REPLACE(REPLACE(issue_json, 'Пополняется `Forge`', 'пополняется Гефестом'), 'Пополняется Forge', 'пополняется Гефестом')
WHERE instr(issue_json, 'Пополняется') > 0 AND instr(issue_json, 'Forge') > 0;

UPDATE forge_log SET
    task = REPLACE(REPLACE(task, 'Пополняется `Forge`', 'пополняется Гефестом'), 'Пополняется Forge', 'пополняется Гефестом'),
    log_path = REPLACE(REPLACE(log_path, 'Пополняется `Forge`', 'пополняется Гефестом'), 'Пополняется Forge', 'пополняется Гефестом')
WHERE instr(task, 'Пополняется') > 0 AND instr(task, 'Forge') > 0
   OR instr(log_path, 'Пополняется') > 0 AND instr(log_path, 'Forge') > 0;

UPDATE artifact SET
    path = REPLACE(REPLACE(path, 'Пополняется `Forge`', 'пополняется Гефестом'), 'Пополняется Forge', 'пополняется Гефестом')
WHERE instr(path, 'Пополняется') > 0 AND instr(path, 'Forge') > 0;

UPDATE book SET
    name = REPLACE(REPLACE(name, 'Пополняется `Forge`', 'пополняется Гефестом'), 'Пополняется Forge', 'пополняется Гефестом'),
    genres = REPLACE(REPLACE(genres, 'Пополняется `Forge`', 'пополняется Гефестом'), 'Пополняется Forge', 'пополняется Гефестом'),
    rating = REPLACE(REPLACE(rating, 'Пополняется `Forge`', 'пополняется Гефестом'), 'Пополняется Forge', 'пополняется Гефестом')
WHERE instr(name, 'Пополняется') > 0 AND instr(name, 'Forge') > 0
   OR instr(genres, 'Пополняется') > 0 AND instr(genres, 'Forge') > 0
   OR instr(rating, 'Пополняется') > 0 AND instr(rating, 'Forge') > 0;

UPDATE book_chunk SET
    chapter = REPLACE(REPLACE(chapter, 'Пополняется `Forge`', 'пополняется Гефестом'), 'Пополняется Forge', 'пополняется Гефестом')
WHERE instr(chapter, 'Пополняется') > 0 AND instr(chapter, 'Forge') > 0;

UPDATE book_character SET
    name = REPLACE(REPLACE(name, 'Пополняется `Forge`', 'пополняется Гефестом'), 'Пополняется Forge', 'пополняется Гефестом'),
    appearance = REPLACE(REPLACE(appearance, 'Пополняется `Forge`', 'пополняется Гефестом'), 'Пополняется Forge', 'пополняется Гефестом'),
    description = REPLACE(REPLACE(description, 'Пополняется `Forge`', 'пополняется Гефестом'), 'Пополняется Forge', 'пополняется Гефестом')
WHERE instr(name, 'Пополняется') > 0 AND instr(name, 'Forge') > 0
   OR instr(appearance, 'Пополняется') > 0 AND instr(appearance, 'Forge') > 0
   OR instr(description, 'Пополняется') > 0 AND instr(description, 'Forge') > 0;

UPDATE book_character_role SET
    evidence = REPLACE(REPLACE(evidence, 'Пополняется `Forge`', 'пополняется Гефестом'), 'Пополняется Forge', 'пополняется Гефестом')
WHERE instr(evidence, 'Пополняется') > 0 AND instr(evidence, 'Forge') > 0;

UPDATE book_chunk_tag SET
    anchors = REPLACE(REPLACE(anchors, 'Пополняется `Forge`', 'пополняется Гефестом'), 'Пополняется Forge', 'пополняется Гефестом')
WHERE instr(anchors, 'Пополняется') > 0 AND instr(anchors, 'Forge') > 0;

UPDATE book_place SET
    name = REPLACE(REPLACE(name, 'Пополняется `Forge`', 'пополняется Гефестом'), 'Пополняется Forge', 'пополняется Гефестом'),
    aliases = REPLACE(REPLACE(aliases, 'Пополняется `Forge`', 'пополняется Гефестом'), 'Пополняется Forge', 'пополняется Гефестом')
WHERE instr(name, 'Пополняется') > 0 AND instr(name, 'Forge') > 0
   OR instr(aliases, 'Пополняется') > 0 AND instr(aliases, 'Forge') > 0;

UPDATE book_scene SET
    time_marker = REPLACE(REPLACE(time_marker, 'Пополняется `Forge`', 'пополняется Гефестом'), 'Пополняется Forge', 'пополняется Гефестом'),
    participants = REPLACE(REPLACE(participants, 'Пополняется `Forge`', 'пополняется Гефестом'), 'Пополняется Forge', 'пополняется Гефестом'),
    tags = REPLACE(REPLACE(tags, 'Пополняется `Forge`', 'пополняется Гефестом'), 'Пополняется Forge', 'пополняется Гефестом'),
    emotion = REPLACE(REPLACE(emotion, 'Пополняется `Forge`', 'пополняется Гефестом'), 'Пополняется Forge', 'пополняется Гефестом')
WHERE instr(time_marker, 'Пополняется') > 0 AND instr(time_marker, 'Forge') > 0
   OR instr(participants, 'Пополняется') > 0 AND instr(participants, 'Forge') > 0
   OR instr(tags, 'Пополняется') > 0 AND instr(tags, 'Forge') > 0
   OR instr(emotion, 'Пополняется') > 0 AND instr(emotion, 'Forge') > 0;

UPDATE book_style SET
    author = REPLACE(REPLACE(author, 'Пополняется `Forge`', 'пополняется Гефестом'), 'Пополняется Forge', 'пополняется Гефестом'),
    features = REPLACE(REPLACE(features, 'Пополняется `Forge`', 'пополняется Гефестом'), 'Пополняется Forge', 'пополняется Гефестом'),
    labels = REPLACE(REPLACE(labels, 'Пополняется `Forge`', 'пополняется Гефестом'), 'Пополняется Forge', 'пополняется Гефестом'),
    drama_profile = REPLACE(REPLACE(drama_profile, 'Пополняется `Forge`', 'пополняется Гефестом'), 'Пополняется Forge', 'пополняется Гефестом')
WHERE instr(author, 'Пополняется') > 0 AND instr(author, 'Forge') > 0
   OR instr(features, 'Пополняется') > 0 AND instr(features, 'Forge') > 0
   OR instr(labels, 'Пополняется') > 0 AND instr(labels, 'Forge') > 0
   OR instr(drama_profile, 'Пополняется') > 0 AND instr(drama_profile, 'Forge') > 0;

UPDATE dict_staging SET
    lemma = REPLACE(REPLACE(lemma, 'Пополняется `Forge`', 'пополняется Гефестом'), 'Пополняется Forge', 'пополняется Гефестом'),
    evidence = REPLACE(REPLACE(evidence, 'Пополняется `Forge`', 'пополняется Гефестом'), 'Пополняется Forge', 'пополняется Гефестом')
WHERE instr(lemma, 'Пополняется') > 0 AND instr(lemma, 'Forge') > 0
   OR instr(evidence, 'Пополняется') > 0 AND instr(evidence, 'Forge') > 0;
