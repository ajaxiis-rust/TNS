// Детектор отказов модели (T-12-06): сверяет форму ответа, а не отдельные
// слова. Ответ признаётся отказом, если он короткий и в его начале есть
// фраза-маркер из packs/<lang>/refusals.txt.
package censor

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// refusalFile — имя файла фраз-отказов пакета (T-12-06): по одной фразе-маркеру
// в строке.
const refusalFile = "refusals.txt"

// maxRefusalRunes — детектор применяется только к короткому ответу: длинный
// ответ модели (более 600 знаков) отказом не считается.
const maxRefusalRunes = 600

// maxRefusalHead — маркеры ищутся только в первых 300 знаках ответа.
const maxRefusalHead = 300

// Detector — детектор отказов модели: хранит фразы-маркеры отказа и сверяет по
// ним форму ответа.
type Detector struct {
	markers []string
}

// NewDetector собирает детектор из фраз-маркеров; пустые строки пропускаются.
// Сравнение нечувствительно к регистру.
func NewDetector(markers []string) *Detector {
	var out []string
	for _, m := range markers {
		m = strings.TrimSpace(m)
		if m == "" {
			continue
		}
		out = append(out, m)
	}
	return &Detector{markers: out}
}

// Refusal возвращает (true, совпавшая фраза), если text — отказ модели: ответ
// короткий, и в первых 300 знаках есть маркер. Совпавшая фраза — первый маркер
// в порядке словаря; пустая строка, если отказа нет.
func (d *Detector) Refusal(text string) (bool, string) {
	runes := []rune(text)
	if len(runes) > maxRefusalRunes {
		return false, ""
	}
	head := runes
	if len(head) > maxRefusalHead {
		head = head[:maxRefusalHead]
	}
	lower := strings.ToLower(string(head))
	for _, m := range d.markers {
		if strings.Contains(lower, strings.ToLower(m)) {
			return true, m
		}
	}
	return false, ""
}

// LoadRefusals читает фразы-маркеры отказов из refusals.txt пакета: по одной
// фразе в строке, пустые строки пропускаются.
func LoadRefusals(packDir string) ([]string, error) {
	f, err := os.Open(filepath.Join(packDir, refusalFile))
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		out = append(out, line)
	}
	return out, sc.Err()
}

// IsRefusal — единый детектор отказов пакета (T-12-06): читает
// packs/<lang>/refusals.txt и проверяет форму ответа text. Совпавшая фраза —
// маркер, по которому ответ признан отказом; пустая строка — отказ не найден.
// Словарь недоступен — отказ не найден (false, "").
func IsRefusal(text, lang string) (bool, string) {
	root := engineRootOnce()
	if root == "" {
		return false, ""
	}
	markers, err := LoadRefusals(filepath.Join(root, "packs", lang))
	if err != nil {
		return false, ""
	}
	return NewDetector(markers).Refusal(text)
}
