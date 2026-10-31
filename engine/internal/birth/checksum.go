package birth

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// errBaseChecksumMismatch — сумма базы языка не совпала с .sha256 (TUN-9,
// T-10-04). Выделена отдельной ошибкой, чтобы вызов мог отличить несовпадение
// от отсутствия файла суммы и пометить событие в журнал.
var errBaseChecksumMismatch = errors.New("база языка повреждена (sha256)")

// VerifyBaseChecksum сверяет SHA-256 файла базы с суммой из dbPath+".sha256"
// (формат: голый hex, возможен '\n'). Файл суммы отсутствует или не читается —
// ошибка «нет контрольной суммы базы <путь>»; сумма не совпала или не hex —
// errBaseChecksumMismatch.
func VerifyBaseChecksum(dbPath string) error {
	raw, err := os.ReadFile(dbPath + ".sha256")
	if err != nil {
		return fmt.Errorf("нет контрольной суммы базы %s", dbPath)
	}
	want, err := hex.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil {
		return errBaseChecksumMismatch
	}

	f, err := os.Open(dbPath)
	if err != nil {
		return fmt.Errorf("читать базу языка для контрольной суммы: %w", err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("читать базу языка для контрольной суммы: %w", err)
	}
	if !bytes.Equal(want, h.Sum(nil)) {
		return errBaseChecksumMismatch
	}
	return nil
}
