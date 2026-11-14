package textlimit

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEstimateLanguages — русский текст на той же длине даёт вдвое больше
// токенов, латиница — прежняя оценка рун/4.
func TestEstimateLanguages(t *testing.T) {
	ru := strings.Repeat("я", 100)
	en := strings.Repeat("a", 100)
	if got, want := Estimate(ru), 50; got != want {
		t.Fatalf("Estimate(100 кириллических) = %d, want %d", got, want)
	}
	if got, want := Estimate(en), 25; got != want {
		t.Fatalf("Estimate(100 латинских) = %d, want %d", got, want)
	}
	if Estimate("") != 0 {
		t.Fatalf("Estimate(\"\") = %d, want 0", Estimate(""))
	}
	// ё входит в диапазон кириллицы; округление вверх.
	if got, want := Estimate("ёж"), 1; got != want {
		t.Fatalf("Estimate(\"ёж\") = %d, want %d", got, want)
	}
	if got, want := Estimate("abcd"), 1; got != want {
		t.Fatalf("Estimate(\"abcd\") = %d, want %d", got, want)
	}
	if got, want := Estimate("abcde"), 2; got != want {
		t.Fatalf("Estimate(\"abcde\") = %d, want %d", got, want)
	}
}

// TestCheckBoundary — 2000 токенов проходят, 2001 — отказ с подсказкой.
func TestCheckBoundary(t *testing.T) {
	if err := Check(KindPlayerMessage, strings.Repeat("я", 4000), 0); err != nil {
		t.Fatalf("ровно 2000 токенов отвергнуты: %v", err)
	}
	if err := Check(KindPlayerMessage, strings.Repeat("a", 8000), 0); err != nil {
		t.Fatalf("ровно 2000 латинских токенов отвергнуты: %v", err)
	}

	err := Check(KindPlayerMessage, strings.Repeat("я", 4001), 0)
	if err == nil {
		t.Fatal("2001 токен пропущен")
	}
	var tooLong *TooLongError
	if !errors.As(err, &tooLong) {
		t.Fatalf("ошибка не *TooLongError: %T", err)
	}
	if tooLong.Tokens != 2001 || tooLong.Limit != DefaultLimit {
		t.Fatalf("Tokens/Limit = %d/%d, want 2001/%d", tooLong.Tokens, tooLong.Limit, DefaultLimit)
	}
	want := "Реплика игрока длиннее 2000 токенов (≈2001): сократите текст"
	if got := err.Error(); got != want {
		t.Fatalf("подсказка = %q, want %q", got, want)
	}
	if Hint(KindWorldDescription, 5, 10) != "Описание мира длиннее 10 токенов (≈5): сократите текст" {
		t.Fatalf("Hint для описания мира: %q", Hint(KindWorldDescription, 5, 10))
	}
}

// TestCheckCustomLimit — явный лимит главнее умолчания.
func TestCheckCustomLimit(t *testing.T) {
	if err := Check(KindWorldDescription, strings.Repeat("a", 400), 100); err != nil {
		t.Fatalf("400 единиц = 100 токенов отвергнуты при лимите 100: %v", err)
	}
	if err := Check(KindWorldDescription, strings.Repeat("a", 404), 100); err == nil {
		t.Fatal("101 токен пропущен при лимите 100")
	}
}

// TestLoadConfig — секция limits, её отсутствие и нечисловые значения.
func TestLoadConfig(t *testing.T) {
	dir := t.TempDir()

	withSection := filepath.Join(dir, "with.yaml")
	body := "web:\n  host: 127.0.0.1\n\nlimits:\n  player_message_tokens: 1500\n  world_description_tokens: 900\n"
	if err := os.WriteFile(withSection, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(withSection)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.PlayerMessage != 1500 || cfg.WorldDescription != 900 {
		t.Fatalf("limits = %+v, want 1500/900", cfg)
	}

	noSection := filepath.Join(dir, "plain.yaml")
	if err := os.WriteFile(noSection, []byte("web:\n  host: 127.0.0.1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err = LoadConfig(noSection)
	if err != nil {
		t.Fatalf("LoadConfig без секции: %v", err)
	}
	if cfg != DefaultLimits() {
		t.Fatalf("limits без секции = %+v, want %+v", cfg, DefaultLimits())
	}

	partial := filepath.Join(dir, "partial.yaml")
	partialBody := "limits:\n  player_message_tokens: 1200\n  world_description_tokens: много\n"
	if err := os.WriteFile(partial, []byte(partialBody), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err = LoadConfig(partial)
	if err != nil {
		t.Fatalf("LoadConfig частичной секции: %v", err)
	}
	if cfg.PlayerMessage != 1200 || cfg.WorldDescription != DefaultLimit {
		t.Fatalf("частичные limits = %+v, want 1200/%d", cfg, DefaultLimit)
	}

	if _, err := LoadConfig(filepath.Join(dir, "absent.yaml")); err == nil {
		t.Fatal("отсутствующий файл не дал ошибку")
	}
}
