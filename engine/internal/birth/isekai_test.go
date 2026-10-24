package birth

import (
	"database/sql"
	"testing"

	"tns65/internal/testkit"
)

func TestIsekaiReplay(t *testing.T) {
	dir := t.TempDir()
	first := validAncketa()

	res, err := Birth(first, Config{WorldsRoot: dir, BaseDBPath: testBaseDB(t, dir), Embed: testkit.NewFakeEmbed()})
	if err != nil {
		t.Fatalf("первое рождение: %v", err)
	}

	// Состояние мира до переигровки: 96 карточек и значения пяти замороженных
	// ключей world_meta.
	before := openReader(t, res.WorldDB)
	frozenBefore := worldMetaValues(t, before, WorldFrozenFields)
	var cardsBefore int
	if err := before.QueryRow(`SELECT COUNT(*) FROM characters`).Scan(&cardsBefore); err != nil {
		t.Fatalf("characters count до переигровки: %v", err)
	}
	if cardsBefore != 96 {
		t.Fatalf("карточек до переигровки: %d, want 96", cardsBefore)
	}

	// Новая анкета: тот же слаг, новые синопсис и герой; замороженные поля
	// нарочно другие — переигровка обязана их проигнорировать.
	replay := validAncketa()
	replay.Synopsis = "Новый герой приходит в забытый мир. Он меняет его судьбу. Финал иной."
	replay.HeroBirth = "аристократ"
	replay.Description = "Другое описание."
	replay.SocialOrder = "демократия"
	replay.EconomyMods = []string{"золотой стандарт"}
	replay.Magic = "обильная"
	replay.Censorship = "18+"

	if _, err := Birth(replay, Config{WorldsRoot: dir, Isekai: true}); err != nil {
		t.Fatalf("переигровка: %v", err)
	}

	after := openReader(t, res.WorldDB)

	// Пачка не перегенерировалась: карточек по-прежнему 96.
	var cardsAfter int
	if err := after.QueryRow(`SELECT COUNT(*) FROM characters`).Scan(&cardsAfter); err != nil {
		t.Fatalf("characters count после переигровки: %v", err)
	}
	if cardsAfter != 96 {
		t.Fatalf("карточек после переигровки: %d, want 96", cardsAfter)
	}

	// Пять замороженных ключей world_meta не изменились.
	frozenAfter := worldMetaValues(t, after, WorldFrozenFields)
	for _, k := range WorldFrozenFields {
		if frozenAfter[k] != frozenBefore[k] {
			t.Fatalf("замороженный ключ %q изменился: было %q, стало %q", k, frozenBefore[k], frozenAfter[k])
		}
	}

	// В истории мира появилось событие past_hero.
	var pastHero int
	if err := after.QueryRow(`SELECT COUNT(*) FROM events WHERE kind = 'past_hero'`).Scan(&pastHero); err != nil {
		t.Fatalf("events past_hero: %v", err)
	}
	if pastHero == 0 {
		t.Fatal("событие past_hero не записано в историю мира")
	}

	// Открыта новая сессия и первая глава новой книги.
	var sessions int
	if err := after.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&sessions); err != nil {
		t.Fatalf("sessions: %v", err)
	}
	if sessions != 1 {
		t.Fatalf("сессий: %d, want 1", sessions)
	}
	var chapters int
	if err := after.QueryRow(`SELECT COUNT(*) FROM book_chapters`).Scan(&chapters); err != nil {
		t.Fatalf("book_chapters: %v", err)
	}
	if chapters != 1 {
		t.Fatalf("глав: %d, want 1", chapters)
	}
}

// worldMetaValues читает значения world_meta по ключам как есть (JSON-строки),
// не разбирая тип значения.
func worldMetaValues(t *testing.T, d *sql.DB, keys []string) map[string]string {
	t.Helper()
	out := make(map[string]string, len(keys))
	for _, k := range keys {
		var v string
		if err := d.QueryRow(`SELECT value FROM world_meta WHERE "key" = ?`, k).Scan(&v); err != nil {
			t.Fatalf("world_meta %q: %v", k, err)
		}
		out[k] = v
	}
	return out
}
