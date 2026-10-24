package turn

import (
	"strings"
	"sync/atomic"
	"unicode"

	"tns65/internal/lemmarpc"
)

// Lemmatizer — минимальный контракт клиента лемматизации (T-01-09): разбор
// сегмента на токены с леммой. Реализация в проде — internal/lemmarpc.Client,
// в тестах подменяется.
type Lemmatizer interface {
	Lemmatize(text string) ([]lemmarpc.Token, error)
}

// Segment — сегмент резки реплики: текст, леммы и вектор смысла (T-03-02, шаг 2).
type Segment struct {
	Text   string
	Lemmas []string
	Vector []float32 // L2-нормированный, dim = models.EmbedDim.
}

// WordVector — ранний вектор слова (шаг 3): слово векторизуется сразу по границе
// «пробел или знак»; промежуточный вектор коммит не снимает.
type WordVector struct {
	Word   string
	Vector []float32
}

// Cut — результат резки, лемматизации и векторизации реплики. Все векторы хода
// связаны TurnID в памяти хода; в таблицу vectors они попадут только после
// подтверждения (T-03-05).
type Cut struct {
	TurnID   int64
	Segments []Segment
	Whole    []float32 // вектор целой реплики.
	Words    []WordVector
}

// turnSeq — источник номеров ходов в памяти процесса.
var turnSeq atomic.Int64

// nextTurnID — уникальный номер хода: связывает векторы одного хода (шаг 2).
func nextTurnID() int64 {
	return turnSeq.Add(1)
}

// cutReplica режет реплику на сегменты по знакам . ! ? … ; : и отбрасывает пустые
// сегменты (шаг 1).
func cutReplica(text string) []string {
	fields := strings.FieldsFunc(text, func(r rune) bool {
		switch r {
		case '.', '!', '?', '…', ';', ':':
			return true
		}
		return false
	})
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	return out
}

// splitWords режет реплику на слова для ранних векторов (шаг 3): граница — пробел
// или знак препинания.
func splitWords(text string) []string {
	return strings.FieldsFunc(text, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsPunct(r)
	})
}
