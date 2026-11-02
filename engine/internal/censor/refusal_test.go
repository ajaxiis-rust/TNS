package censor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixtureRefusals пишет фикстуру refusals.txt с фразами-маркерами из целевой
// механики (P12.2) и возвращает каталог пакета.
func fixtureRefusals(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	content := "как ИИ\nязыковая модель\nне могу помочь с этим запросом\nпротив политики\nизвините, но я не могу\n"
	if err := os.WriteFile(filepath.Join(dir, refusalFile), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestRefusal — таблица положительных и отрицательных примеров (T-12-06,
// «Проверка»): отказ по форме ответа, реплика героя отказом не считается.
func TestRefusal(t *testing.T) {
	packDir := fixtureRefusals(t)
	markers, err := LoadRefusals(packDir)
	if err != nil {
		t.Fatalf("LoadRefusals: %v", err)
	}
	d := NewDetector(markers)

	long := "как ИИ " + strings.Repeat("текст ", 100)                    // 607 знаков — не короткий ответ.
	afterHead := strings.Repeat("а", maxRefusalHead) + "против политики" // маркер за первыми 300 знаками.

	cases := []struct {
		text   string
		want   bool
		phrase string // точная совпавшая фраза; пустая — только признак.
	}{
		// Положительные примеры — отказы модели.
		{"Извините, но я не могу помочь с этим запросом", true, "не могу помочь с этим запросом"},
		{"Как ИИ, я не могу отвечать на этот вопрос", true, "как ИИ"},
		{"Я языковая модель и не вправе это обсуждать", true, "языковая модель"},
		{"Это против политики безопасности", true, "против политики"},
		// Отрицательные примеры — реплики героя, обычная проза, форма ответа.
		{"Я не могу тебе помочь, брат", false, ""},
		{"Не могу тебе помочь, сам разбирайся", false, ""},
		{"Добрый человек идёт по лесной дороге", false, ""},
		{long, false, ""},
		{afterHead, false, ""},
	}
	for _, tc := range cases {
		got, phrase := d.Refusal(tc.text)
		if got != tc.want {
			t.Errorf("%q: отказ=%v phrase=%q, want отказ=%v", tc.text, got, phrase, tc.want)
			continue
		}
		if tc.phrase != "" && phrase != tc.phrase {
			t.Errorf("%q: совпавшая фраза %q, want %q", tc.text, phrase, tc.phrase)
		}
		if got && phrase == "" {
			t.Errorf("%q: отказ найден, но совпавшая фраза пуста", tc.text)
		}
		if !got && phrase != "" {
			t.Errorf("%q: отказа нет, но вернулась фраза %q", tc.text, phrase)
		}
	}
}

// TestRealRefusals — приёмка T-12-06 на реальном packs/ru/refusals.txt: отказ
// модели ловится, реплика героя отказом не считается.
func TestRealRefusals(t *testing.T) {
	root := findEngineRoot()
	if root == "" {
		t.Skip("корень движка не найден — пакет недоступен")
	}
	packDir := filepath.Join(root, "packs", "ru")
	markers, err := LoadRefusals(packDir)
	if err != nil {
		t.Fatalf("LoadRefusals: %v", err)
	}
	if len(markers) == 0 {
		t.Fatal("refusals.txt пакета пуст")
	}
	d := NewDetector(markers)

	if ok, phrase := d.Refusal("Извините, но я не могу помочь с этим запросом"); !ok {
		t.Fatalf("приёмка: отказ не найден, phrase=%q", phrase)
	}
	if ok, phrase := d.Refusal("Я не могу тебе помочь, брат"); ok {
		t.Fatalf("приёмка: реплика героя признана отказом, phrase=%q", phrase)
	}
}
