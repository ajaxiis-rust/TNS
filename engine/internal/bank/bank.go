// Package bank — Челнок: банк рутины литературной базы. Выбор записи из
// bank_pool по тегам сцены идёт детерминированным кодом, без вызова слота
// генерации (LT-08.3); банк хранит форму, мир подставляет факты (LT-08.5).
package bank

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"tns65/internal/db"
)

// Slot — слот генерации прозы (T-02-14). Челнок его не вызывает: выбор идёт
// кодом, без дрейфа модели; ссылка хранится, чтобы проверять «ноль вызовов».
type Slot interface {
	Generate(ctx context.Context, role, prompt string) (string, error)
}

// Clock — источник времени бюджета выборки; testkit.Clock удовлетворяет контракту.
type Clock interface {
	Now() time.Time
}

// Binder — подстановка фактов мира в форму записи банка (LT-08.5): банк хранит
// форму с токенами-заменителями, мир — факты (имена, цены, места).
type Binder interface {
	Bind(text string) string
}

// Budget — бюджет выборки записи банка (T-03-07, шаг 1): Челнок отвечает быстро,
// в отличие от Ткача.
const Budget = 100 * time.Millisecond

// recentWindow — сколько последних наборов тегов помнит банк для «повторяемого»
// (T-03-07, шаг 1): тот же набор тегов в пределах окна выдаёт ту же запись.
const recentWindow = 3

// Ошибки Челнока.
var (
	// ErrNoRecord — в банке нет записи, подходящей тегам сцены.
	ErrNoRecord = errors.New("bank: нет записи под теги сцены")
	// ErrNoReader — читатель литературной базы не задан.
	ErrNoReader = errors.New("bank: читатель литературной базы не задан")
)

// Record — запись пула банка bank_pool литературной базы.
type Record struct {
	ID   int64
	Kind string
	Text string
	Tags string
}

// recentPick — запомненный выбор по набору тегов для «повторяемого».
type recentPick struct {
	key string
	rec Record
}

// Bank — Челнок: выбор записи из bank_pool по тегам сцены.
type Bank struct {
	reader *db.Handle // литературная база; только чтение (bank_pool пишет forge).
	clock  Clock
	slot   Slot // не вызывается; присутствует для проверки «ноль вызовов слота».
	binder Binder
	recent []recentPick
}

// New собирает Челнок над литературной базой reader (чтение), часами clock,
// слотом генерации slot и биндером фактов binder. clock и binder могут быть nil:
// тогда время берётся реальное, а форма выдаётся без подстановки фактов.
func New(reader *db.Handle, clock Clock, slot Slot, binder Binder) *Bank {
	return &Bank{reader: reader, clock: clock, slot: slot, binder: binder}
}

// Pick выбирает запись банка по тегам сцены tags и возвращает её с подставленными
// фактами. Выбор детерминированный, без вызова слота генерации, в бюджете Budget.
// Повторяемое: тот же набор тегов в пределах recentWindow последних ходов выдаёт
// ту же запись.
func (b *Bank) Pick(tags []string) (Record, error) {
	if b.reader == nil {
		return Record{}, ErrNoReader
	}
	start := b.now()

	rec, err := b.pick(tags)
	if err != nil {
		return Record{}, err
	}

	if elapsed := b.now().Sub(start); elapsed > Budget {
		return Record{}, fmt.Errorf("bank: выборка превысила бюджет: %v > %v", elapsed, Budget)
	}
	return rec, nil
}

// pick — выбор с памятью последних ходов: попадание в окно возвращает прошлый
// выбор без нового чтения; иначе — детерминированный выбор и запоминание.
func (b *Bank) pick(tags []string) (Record, error) {
	key := canonicalTags(tags)
	if i := b.recentIndex(key); i >= 0 {
		return b.recent[i].rec, nil
	}

	rec, err := b.selectRecord(tags)
	if err != nil {
		return Record{}, err
	}
	rec = b.bind(rec)
	b.remember(key, rec)
	return rec, nil
}

// selectRecord — детерминированный выбор записи: среди записей, чьи теги
// пересекаются с тегами сцены, побеждает запись с наибольшим числом совпадений;
// при равенстве — меньший id.
func (b *Bank) selectRecord(tags []string) (Record, error) {
	rows, err := b.reader.Query(`SELECT id, kind, text, tags FROM bank_pool`)
	if err != nil {
		return Record{}, err
	}
	defer rows.Close()

	want := tagSet(tags)
	var best Record
	bestScore := -1
	for rows.Next() {
		var rec Record
		if err := rows.Scan(&rec.ID, &rec.Kind, &rec.Text, &rec.Tags); err != nil {
			return Record{}, err
		}
		score := overlapScore(want, parseTags(rec.Tags))
		switch {
		case score > bestScore:
			bestScore = score
			best = rec
		case score == bestScore && bestScore > 0 && rec.ID < best.ID:
			best = rec
		}
	}
	if err := rows.Err(); err != nil {
		return Record{}, err
	}
	if bestScore <= 0 {
		return Record{}, ErrNoRecord
	}
	return best, nil
}

// bind подставляет факты мира в форму записи биндером; без биндера — форма как есть.
func (b *Bank) bind(rec Record) Record {
	if b.binder != nil {
		rec.Text = b.binder.Bind(rec.Text)
	}
	return rec
}

// now — текущее время часов; nil — реальное время.
func (b *Bank) now() time.Time {
	if b.clock != nil {
		return b.clock.Now()
	}
	return time.Now()
}

// recentIndex — номер в окне памяти по каноническому ключу; −1 — нет.
func (b *Bank) recentIndex(key string) int {
	for i := len(b.recent) - 1; i >= 0; i-- {
		if b.recent[i].key == key {
			return i
		}
	}
	return -1
}

// remember кладёт выбор в окно памяти и держит его длиной не более recentWindow.
func (b *Bank) remember(key string, rec Record) {
	b.recent = append(b.recent, recentPick{key: key, rec: rec})
	if n := len(b.recent); n > recentWindow {
		b.recent = b.recent[n-recentWindow:]
	}
}

// canonicalTags — каноническая форма набора тегов для сравнения «повторяемого».
func canonicalTags(tags []string) string {
	set := tagSet(tags)
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, ",")
}

// tagSet — нормализованный набор тегов: без пустых, в нижнем регистре.
func tagSet(tags []string) map[string]bool {
	out := make(map[string]bool)
	for _, t := range tags {
		if t = strings.TrimSpace(strings.ToLower(t)); t != "" {
			out[t] = true
		}
	}
	return out
}

// parseTags — теги записи банка: поле tags — список через запятую.
func parseTags(s string) map[string]bool {
	return tagSet(strings.Split(s, ","))
}

// overlapScore — число тегов сцены, встретившихся в тегах записи.
func overlapScore(want, have map[string]bool) int {
	n := 0
	for k := range want {
		if have[k] {
			n++
		}
	}
	return n
}
