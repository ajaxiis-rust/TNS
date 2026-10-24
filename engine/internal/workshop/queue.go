package workshop

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"tns65/internal/db"
)

// Kind — вид заказа мастерской (T-02-11, шаг 1).
type Kind string

// Единый реестр видов заказов. Обязан совпадать с CHECK в
// 0003_workshop_queue.sql: новый вид добавляется правкой CHECK вместе с
// задачей-владельцем.
const (
	KindAnalysis   Kind = "analysis"
	KindForgePlan  Kind = "forge_plan"
	KindComplement Kind = "complement"
	KindHoroscope  Kind = "horoscope"
	KindPythia     Kind = "pythia"
	KindEdit       Kind = "edit"
	KindEpoch      Kind = "epoch"
	KindStyle      Kind = "style"
	KindSediment   Kind = "sediment"
	KindRing       Kind = "ring"
	KindWakeup     Kind = "wakeup"
)

// Kinds — виды заказов в порядке реестра CHECK.
var Kinds = []Kind{
	KindAnalysis, KindForgePlan, KindComplement, KindHoroscope, KindPythia,
	KindEdit, KindEpoch, KindStyle, KindSediment, KindRing, KindWakeup,
}

// Status — статус заказа; меняет только ядро.
type Status string

const (
	StatusQueued    Status = "queued"
	StatusRunning   Status = "running"
	StatusDone      Status = "done"
	StatusCancelled Status = "cancelled"
	StatusFailed    Status = "failed"
)

// Step — ступень заказа (T-05-01 сопоставляет ступени паузы весу работ).
type Step string

const (
	StepLight Step = "light"
	StepHeavy Step = "heavy"
)

// Приоритеты видов (T-02-11, шаг 1): 100 пробуждение, 60 гороскоп, 50 оседание,
// 30 доукомплектация, 20 кольца, 10 стиль. Остальные виды (в т.ч. analysis и
// forge_plan) получают defaultPriority.
const defaultPriority = 50

// KindPriority возвращает приоритет вида заказа по реестру T-02-11.
func KindPriority(k Kind) int {
	switch k {
	case KindWakeup:
		return 100
	case KindHoroscope:
		return 60
	case KindSediment:
		return 50
	case KindComplement:
		return 30
	case KindRing:
		return 20
	case KindStyle:
		return 10
	default:
		return defaultPriority
	}
}

// Provider — пресет слота мастерской (T-02-11, шаг 3).
type Provider string

const (
	ProviderLocal       Provider = "local"
	ProviderGeminiFlash Provider = "gemini-flash"
	ProviderGeminiPro   Provider = "gemini-pro"
)

// DefaultProvider — пресет по умолчанию: local.
const DefaultProvider = ProviderLocal

// Адреса локальных слотов: генерация :5001, эмбеддинги :5002 (шаг 3).
const (
	localGenAddr   = ":5001"
	localEmbedAddr = ":5002"
)

// Preset — заводской пресет слота: провайдер и адреса локальных слотов.
type Preset struct {
	Provider  Provider
	GenAddr   string
	EmbedAddr string
}

// Presets — заводской реестр пресетов (шаг 3): local (дефолт), gemini-flash,
// gemini-pro. Ключи внешних провайдеров вводятся в UI и хранятся в settings.toml
// вне world_db/; локальных адресов у внешних пресетов нет.
func Presets() []Preset {
	return []Preset{
		{Provider: ProviderLocal, GenAddr: localGenAddr, EmbedAddr: localEmbedAddr},
		{Provider: ProviderGeminiFlash},
		{Provider: ProviderGeminiPro},
	}
}

// GenSlot — адрес слота генерации для пресета; внешние пресеты локального адреса
// не имеют. Адрес пишется в model_bindings; сам генератор выбирает slotForProvider.
func GenSlot(p Provider) string {
	if p == ProviderLocal {
		return localGenAddr
	}
	return ""
}

// Slot — слот генерации (T-02-14): по роли и промпту возвращает текст ответа.
// Для пресета local это локальный станок :5001 (адрес — GenSlot); его HTTP-клиент
// подключается задачей T-03-08, до того генерация не выполняется.
type Slot interface {
	Generate(ctx context.Context, role, prompt string) (string, error)
}

// curSlot — подменяемый слот очереди: тесты ставят testkit.FakeSlot через SetSlot.
// В производстве до исполнителя очереди (P4) слота нет — заказы только ставятся
// (T-02-11, шаг 2), выбор ограничивается адресом в model_bindings.
var (
	slotMu  sync.RWMutex
	curSlot Slot
)

// SetSlot подменяет слот очереди (тестовая точка входа T-02-14); nil — заводской.
func SetSlot(s Slot) {
	slotMu.Lock()
	defer slotMu.Unlock()
	curSlot = s
}

// slotForProvider — выбор слота по пресету (T-02-11, шаг 3; T-02-14, шаг 1).
// Подменённый тестом слот обслуживает любой пресет; в производстве до T-03-08
// генератор не подключён, поэтому возвращается nil.
func slotForProvider(_ Provider) Slot {
	slotMu.RLock()
	defer slotMu.RUnlock()
	return curSlot
}

// Enqueue ставит заказ kind в очередь мира хэндлом core (статус queued),
// привязывает его к слоту по пресету через model_bindings (шаг 3) и пробует
// выполнить через слот (T-02-14). Статус заказа меняет только ядро; снаружи заказ
// создаётся только queued.
func Enqueue(h *db.Handle, kind Kind, step Step, provider Provider, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("workshop: payload заказа %s: %w", kind, err)
	}
	res, err := h.Exec("workshop_queue",
		`INSERT INTO workshop_queue (kind, payload_json, priority, status, step, resumable, attempts, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		string(kind), string(raw), KindPriority(kind), string(StatusQueued), string(step), 0, 0, nowUTC())
	if err != nil {
		return err
	}
	if err := bindSlot(h, kind, provider); err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	attempt(h, id, kind, provider, string(raw))
	return nil
}

// attempt пробует выполнить заказ через слот по пресету (T-02-14, шаги 1–2).
// Отказ локального слота не отменяет рождение и не теряет заказ: attempts растёт,
// статус остаётся queued. Успех кладёт ответ в checkpoint_json — до исполнителя
// очереди (P4) заказ всё равно остаётся queued (T-02-11, шаг 2).
func attempt(h *db.Handle, id int64, kind Kind, provider Provider, prompt string) {
	slot := slotForProvider(provider)
	if slot == nil {
		return
	}
	out, err := slot.Generate(context.Background(), string(kind), prompt)
	if err != nil {
		_, _ = h.Exec("workshop_queue",
			`UPDATE workshop_queue SET attempts = attempts + 1 WHERE id = ?`, id)
		return
	}
	_, _ = h.Exec("workshop_queue",
		`UPDATE workshop_queue SET checkpoint_json = ? WHERE id = ?`, out, id)
}

// EnqueueBirthOrders — постановка двух заказов рождения (шаг 2): analysis
// (анализ данных первичного этапа) и forge_plan (план ковки всей книги), оба
// через слот мастерской, пресет по умолчанию local. До исполнителя очереди (P4)
// заказы живут со статусом queued. Повторная постановка того же заказа для того
// же мира не создаёт дубль (шаг 4).
func EnqueueBirthOrders(h *db.Handle) error {
	for _, o := range []struct {
		kind Kind
		step Step
	}{
		{KindAnalysis, StepHeavy},
		{KindForgePlan, StepLight},
	} {
		if err := enqueueOnce(h, o.kind, o.step); err != nil {
			return err
		}
	}
	return nil
}

// enqueueOnce ставит заказ, если заказа того же вида для этого мира ещё нет.
func enqueueOnce(h *db.Handle, kind Kind, step Step) error {
	var n int
	if err := h.QueryRow(`SELECT COUNT(*) FROM workshop_queue WHERE kind = ?`, string(kind)).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	return Enqueue(h, kind, step, DefaultProvider, map[string]string{"kind": string(kind)})
}

// bindSlot привязывает вид заказа к слоту: строка model_bindings role=kind,
// model_ref=адрес слота, provider=пресет. Повторная привязка не дублирует строку.
func bindSlot(h *db.Handle, kind Kind, provider Provider) error {
	var n int
	if err := h.QueryRow(`SELECT COUNT(*) FROM model_bindings WHERE role = ?`, string(kind)).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	if _, err := h.Exec("model_bindings",
		`INSERT INTO model_bindings (role, model_ref, provider, updated_at) VALUES (?, ?, ?, ?)`,
		string(kind), GenSlot(provider), string(provider), nowUTC()); err != nil {
		return err
	}
	return nil
}

func nowUTC() string {
	return time.Now().UTC().Format(time.RFC3339)
}
