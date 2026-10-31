package birth

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tns65/internal/journal"
)

// writeFixtureSHA256 записывает рядом с фикстурной базой её sha256 в формате
// hex\n (тот же, что пишет tns65-bake) — фикстуры проходят контрольную сумму
// T-10-04.
func writeFixtureSHA256(t *testing.T, dbPath string) {
	t.Helper()
	data, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	if err := os.WriteFile(dbPath+".sha256", []byte(hex.EncodeToString(sum[:])+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyBaseChecksum(t *testing.T) {
	dir := t.TempDir()
	basePath := writeLangBaseDB(t, dir, "en")

	// Чистая фикстура проходит.
	if err := VerifyBaseChecksum(basePath); err != nil {
		t.Fatalf("VerifyBaseChecksum(чистая база): %v, want nil", err)
	}

	// Копия с подменённым байтом: сумма .sha256 от исходной базы не совпадает.
	corrupt := filepath.Join(dir, "base.en.corrupt.db")
	if err := copyAndCorrupt(t, basePath, corrupt); err != nil {
		t.Fatal(err)
	}
	sum, err := os.ReadFile(basePath + ".sha256")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(corrupt+".sha256", sum, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := VerifyBaseChecksum(corrupt); err == nil {
		t.Fatal("VerifyBaseChecksum(битая база): nil, want error")
	} else if !strings.Contains(err.Error(), "sha256") {
		t.Fatalf("причина без sha256: %q", err.Error())
	}

	// Отказ рождения кодом 2, каталог не создан, событие в журнале.
	worldsRoot := t.TempDir()
	logPath := filepath.Join(dir, "journal.log")
	logger, err := journal.New(logPath)
	if err != nil {
		t.Fatal(err)
	}
	a := validAncketa()
	a.Lang = "en"
	_, err = Birth(a, Config{WorldsRoot: worldsRoot, BaseDBPath: corrupt, Logger: logger})

	var ee *ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("ожидался *ExitError, получен %T: %v", err, err)
	}
	if ee.Code != ExitInvalid {
		t.Fatalf("код отказа: %d, want %d", ee.Code, ExitInvalid)
	}
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("ожидался *ValidationError, получен %T: %v", err, err)
	}
	if len(ve.Fields) != 1 || ve.Fields[0].Field != "lang" {
		t.Fatalf("перечень полей: %+v, want ровно одно поле lang", ve.Fields)
	}
	if !strings.Contains(ve.Fields[0].Reason, "sha256") {
		t.Fatalf("причина: %q, want содержит sha256", ve.Fields[0].Reason)
	}
	worldDir := filepath.Join(worldsRoot, "worlds", a.Slug)
	if _, statErr := os.Stat(worldDir); !os.IsNotExist(statErr) {
		t.Fatalf("каталог мира создан при битой базе: %v", statErr)
	}

	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), journal.EventBaseCorrupt) {
		t.Fatalf("журнал не содержит %q:\n%s", journal.EventBaseCorrupt, raw)
	}
}

// copyAndCorrupt копирует файл src в dst и подменяет первый байт копии, чтобы
// сумма не совпала с исходным .sha256.
func copyAndCorrupt(t *testing.T, src, dst string) error {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return errors.New("фикстурная база пуста")
	}
	data[0] ^= 0xff
	return os.WriteFile(dst, data, 0o644)
}
