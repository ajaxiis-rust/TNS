package turn

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tns65/internal/censor"
)

// TestFlirtRealPack — чат-контур erotic.json (T-18-03, D-11): пороги владельца,
// множитель второго лица, ролевые ремарки, анти-медицина и обнуление сарказмом.
func TestFlirtRealPack(t *testing.T) {
	lex, err := LoadLexicon(filepath.Join("..", "..", "packs", "ru"))
	if err != nil {
		t.Fatalf("LoadLexicon(packs/ru): %v", err)
	}
	if lex.Flirt == nil {
		t.Fatal("чат-контур erotic.json не подключён")
	}
	if lex.Flirt.Meta.FlirtThreshold != 0.75 || lex.Flirt.Meta.EroticaThreshold != 1.0 {
		t.Fatalf("пороги = %+v, ожидались 0.75/1.0", lex.Flirt.Meta)
	}

	cases := []struct {
		name    string
		text    string
		flirt   bool
		erotica bool
	}{
		{"второе лицо усиливает", "Ты такая горячая сегодня.", true, false},
		{"третье лицо нейтрально", "Она такая горячая сегодня.", false, false},
		{"интимная категория", "Хочу тебя.", true, true},
		{"ролевая ремарка", "*поцелуй*", true, true},
		{"без ремарки", "Поцелуй.", true, false},
		{"анти-медицина гасит", "Врач сказал, что это симптом, но хочу тебя.", false, false},
		{"горячий чай", "У меня горячий чай.", false, false},
		{"болит грудь", "У меня болит грудь.", false, false},
		{"сарказм обнуляет", "«Хочу тебя», ну да, конечно.", false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := lex.DetectFlirt(tc.text)
			if v.Flirt != tc.flirt || v.Erotica != tc.erotica {
				t.Fatalf("DetectFlirt(%q) = %+v, want flirt=%v erotica=%v", tc.text, v, tc.flirt, tc.erotica)
			}
			if tc.flirt && len(v.Markers) == 0 {
				t.Errorf("DetectFlirt(%q): нет маркеров", tc.text)
			}
		})
	}

	// Без издёвки тот же текст даёт эротику, с издёвкой — нулевой скор.
	if s := lex.DetectSarcasm("«Хочу тебя», ну да, конечно."); !s.Sarcastic {
		t.Fatalf("сарказм не распознан: %+v", s)
	}
	if v := lex.DetectFlirt("«Хочу тебя», ну да, конечно."); v.Score != 0 {
		t.Errorf("скор при сарказме %g, want 0", v.Score)
	}
}

// TestFlirtDefaults — пустые meta берут пороги владельца; файл без секции chat
// и отсутствие erotic.json оставляют детектор пустым.
func TestFlirtDefaults(t *testing.T) {
	dir := t.TempDir()
	writeLexicon(t, dir)
	content := `{"chat":{"categories":{"playful_flirt":{"severity":1,"items":{"красавчик":0.8}}}}}`
	if err := os.WriteFile(filepath.Join(dir, "erotic.json"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	lex, err := LoadLexicon(dir)
	if err != nil {
		t.Fatalf("LoadLexicon: %v", err)
	}
	if v := lex.DetectFlirt("Красавчик!"); !v.Flirt || v.Erotica {
		t.Fatalf("порог по умолчанию: %+v, want flirt без эротики", v)
	}

	// Файл есть, но чат-контура в нём нет: детектор не взводится.
	plain := t.TempDir()
	writeLexicon(t, plain)
	if err := os.WriteFile(filepath.Join(plain, "erotic.json"), []byte(`{"lemmas":{"секс":3}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	lex, err = LoadLexicon(plain)
	if err != nil {
		t.Fatalf("LoadLexicon(без chat): %v", err)
	}
	if lex.Flirt != nil {
		t.Fatalf("чат-контур без секции chat = %+v, want nil", lex.Flirt)
	}
	if v := lex.DetectFlirt("Хочу тебя."); v.Score != 0 || v.Flirt {
		t.Fatalf("DetectFlirt без словаря = %+v, want пусто", v)
	}

	// Файла erotic.json нет вовсе — лексикон грузится без ошибки.
	absent := t.TempDir()
	writeLexicon(t, absent)
	lex, err = LoadLexicon(absent)
	if err != nil {
		t.Fatalf("LoadLexicon(без erotic.json): %v", err)
	}
	if lex.Flirt != nil {
		t.Fatalf("чат-контур без erotic.json = %+v, want nil", lex.Flirt)
	}
}

// TestFlirtDictionaryGate — чат-контур пакета: категории покрывают severity
// 1–4, а элементы не пересекаются со стоп-листом цензора (W-22.4): обсценная
// лексика остаётся цензору.
func TestFlirtDictionaryGate(t *testing.T) {
	packDir := filepath.Join("..", "..", "packs", "ru")
	lex, err := LoadLexicon(packDir)
	if err != nil {
		t.Fatalf("LoadLexicon(packs/ru): %v", err)
	}
	if lex.Flirt == nil {
		t.Fatal("чат-контур erotic.json не подключён")
	}
	stoplist, err := censor.LoadStoplist(packDir)
	if err != nil {
		t.Fatalf("LoadStoplist: %v", err)
	}
	stop := make(map[string]struct{}, len(stoplist))
	for _, line := range stoplist {
		stop[strings.ToLower(strings.TrimSpace(line))] = struct{}{}
	}

	seen := make(map[int]bool, 4)
	for name, cat := range lex.Flirt.Categories {
		if cat.Severity < 1 || cat.Severity > 4 {
			t.Errorf("категория %q: severity %d вне 1–4", name, cat.Severity)
			continue
		}
		seen[cat.Severity] = true
		for item := range cat.Items {
			for _, word := range strings.Fields(strings.ToLower(item)) {
				if _, bad := stop[word]; bad {
					t.Errorf("категория %q: элемент %q пересекается со стоп-листом", name, item)
				}
			}
		}
	}
	for severity := 1; severity <= 4; severity++ {
		if !seen[severity] {
			t.Errorf("категорий severity %d нет в словаре", severity)
		}
	}
}
