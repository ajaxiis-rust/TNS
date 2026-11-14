package deepread_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tns65/internal/deepread"
)

// TestLemmas — гейт лемма-слоя разбора (W-23.6, приёмка T-14-04): фикстура из
// трёх чанков прогоняется резидентом пакета, падежные формы «леса/лесом/лесу»
// сводятся к одной лемме «лес», часть речи размечена, позиции токенов идут по
// строкам чанка. Без бинарника резидента проверка пропускается (образец —
// TestStoplistGolden в internal/censor).
func TestLemmas(t *testing.T) {
	root := findEngineRoot()
	if root == "" {
		t.Skip("корень движка не найден — гейт резидента недоступен")
	}
	packDir := filepath.Join(root, "packs", "ru")

	if _, err := deepread.OpenRun(t.TempDir()); !errors.Is(err, deepread.ErrNoResident) {
		t.Fatalf("OpenRun без манифеста: %v, ожидалась ErrNoResident", err)
	}

	binary := residentBinary(t, packDir)
	if binary == "" {
		t.Skip("бинарник резидента лемматизатора недоступен")
	}

	run, err := deepread.OpenRun(packDir)
	if err != nil {
		t.Fatalf("OpenRun: %v", err)
	}
	defer func() {
		if err := run.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()

	chunks := []string{
		"Леса стояли тихо, и он шёл лесом к дому.\nЛес шумел.",
		"Он подошёл к лесу и остановился.",
		"Лес был густым, и он слушал его.",
	}
	lemmas := run.ChunkLemmas(chunks)
	if err := run.Err(); err != nil {
		t.Fatalf("ChunkLemmas: %v", err)
	}
	if len(lemmas) != len(chunks) {
		t.Fatalf("чанков разобрано %d, ожидалось %d", len(lemmas), len(chunks))
	}

	// Все вхождения падежных форм сводятся к одной лемме.
	targets := []string{"леса", "лесом", "лесу"}
	want := make(map[string]bool, len(targets))
	for _, form := range targets {
		want[form] = false
	}
	for ci, chunkLemmas := range lemmas {
		if len(chunkLemmas) == 0 {
			t.Fatalf("чанк %d: нет токенов", ci)
		}
		for _, l := range chunkLemmas {
			form := strings.ToLower(l.Form)
			if _, ok := want[form]; !ok {
				continue
			}
			want[form] = true
			if l.Lemma != "лес" {
				t.Errorf("чанк %d, форма %q: лемма %q, want «лес»", ci, l.Form, l.Lemma)
			}
			if l.POS != "NOUN" {
				t.Errorf("чанк %d, форма %q: часть речи %q, want NOUN", ci, l.Form, l.POS)
			}
		}
	}
	for form, seen := range want {
		if !seen {
			t.Errorf("форма %q не найдена в разборе", form)
		}
	}

	// Лемма «лес» размечена и по позиции: первая строка чанка — номер 1,
	// вторая — номер 2, позиции токенов в строке идут с 0 без пропусков.
	line1 := lemmas[0]
	if line1[0].Line != 1 || line1[0].Index != 0 {
		t.Errorf("первый токен: позиция (%d,%d), want (1,0)", line1[0].Line, line1[0].Index)
	}
	secondLine := false
	for _, l := range line1 {
		if l.Line == 2 {
			secondLine = true
		}
	}
	if !secondLine {
		t.Errorf("вторая строка чанка не разобрана: %v", line1)
	}

	// Lemmatize на одной строке: позиции токенов непрерывны, лемма и часть речи
	// заполнены у знаменательного слова.
	single, err := run.Lemmatize("Лес")
	if err != nil {
		t.Fatalf("Lemmatize: %v", err)
	}
	if len(single) == 0 {
		t.Fatal("Lemmatize «Лес»: нет токенов")
	}
	if got := single[0]; got.Lemma != "лес" || got.POS != "NOUN" || got.Line != 1 || got.Index != 0 {
		t.Errorf("Lemmatize «Лес»: %+v, want лемма «лес», POS NOUN, позиция (1,0)", got)
	}

	// Пустой текст не поднимает ошибок и не даёт токенов.
	if empty, err := run.Lemmatize(" \n\t"); err != nil || len(empty) != 0 {
		t.Errorf("Lemmatize пустого текста: %d токенов, err=%v", len(empty), err)
	}
}

// residentBinary возвращает путь бинарника резидента лемматизатора из
// manifest.json пакета или пустую строку, если файла нет (гейт пропускается).
func residentBinary(t *testing.T, packDir string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(packDir, "manifest.json"))
	if err != nil {
		return ""
	}
	var m struct {
		Lemmatizer struct {
			Binary string `json:"binary"`
		} `json:"lemmatizer"`
	}
	if err := json.Unmarshal(raw, &m); err != nil || m.Lemmatizer.Binary == "" {
		return ""
	}
	binary := filepath.Join(packDir, filepath.FromSlash(m.Lemmatizer.Binary))
	if _, err := os.Stat(binary); err != nil {
		return ""
	}
	return binary
}
