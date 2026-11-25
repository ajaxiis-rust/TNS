package paganel

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"tns65/internal/journal"
	"tns65/internal/models"
	"tns65/internal/prompts"
	"tns65/internal/providercfg"
)

// RoleScout — роль глубокой разведки в журнале вызовов слота (FIX-4, B-14.4).
const RoleScout = "scout"

// ScoutResultSummaryLen — длина краткой сводки результата в журнале вызовов.
const ScoutResultSummaryLen = 200

// ErrScoutNoSlot — слот разведки не подключён (нет провайдера/ключа).
var ErrScoutNoSlot = errors.New("paganel: слот разведки не подключён")

// ErrScoutNoAnswer — слот закрыл поток без текста: пустой ответ не пишется.
var ErrScoutNoAnswer = errors.New("paganel: разведка вернула пустой ответ")

// ScoutSlot — слот модели для разведки: стриминг чанков и метаданные провайдера
// для журнала вызовов (FIX-4). Реализации — models.OpenAIClient,
// models.ProviderClient и models.Weaver; в тестах подменяется.
type ScoutSlot interface {
	Stream(ctx context.Context, prompt string) (<-chan string, error)
	ProviderName() string
	LimitRemaining() string
}

// DeepScoutConfig — настройки живой глубокой разведки (B-14.4): бюджет и
// тайм-аут — как у разведки в игре (W-10.3, W-10.4), журнал вызовов и журнал
// станка — общие с остальными слотами.
type DeepScoutConfig struct {
	Provider  string          // имя провайдера из providers.toml (роль журнала).
	Endpoint  string          // адрес провайдера для журнала (base_url).
	Budget    int             // вызовов на прогон; 0 — ScoutBudgetDefault.
	Timeout   time.Duration   // тайм-аут одного вызова; 0 — ScoutTimeoutDefault.
	UsagePath string          // журнал вызовов; пусто — models.CallJournalPath.
	Logger    *journal.Logger // журнал станка; может быть nil.
}

// DeepScout — живая глубокая разведка реальными вызовами модели (DEC-4,
// B-14.4): бюджет, тайм-аут на вызов и обязательное логирование каждого вызова,
// включая отказ и тайм-аут. Пропуск разведки не ошибка вызывающего: рождение и
// ход не блокируются (B-14.3). Клиент переиспользуется разведкой в игре (Э5).
type DeepScout struct {
	cfg  DeepScoutConfig
	slot ScoutSlot

	mu    sync.Mutex
	spent int
}

// NewDeepScout создаёт разведку с настройками cfg и слотом slot. Нулевые
// бюджет и тайм-аут заменяются заводскими значениями, пустой журнал вызовов —
// models.CallJournalPath.
func NewDeepScout(cfg DeepScoutConfig, slot ScoutSlot) *DeepScout {
	if cfg.Budget <= 0 {
		cfg.Budget = ScoutBudgetDefault
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = ScoutTimeoutDefault
	}
	if cfg.UsagePath == "" {
		cfg.UsagePath = models.CallJournalPath
	}
	return &DeepScout{cfg: cfg, slot: slot}
}

// Provider — имя провайдера разведки (для журнала и отчёта).
func (s *DeepScout) Provider() string { return s.cfg.Provider }

// Spent — израсходованные вызовы разведки этого прогона.
func (s *DeepScout) Spent() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.spent
}

// Run выполняет разведку по темам в пределах бюджета. Ошибка одного вызова не
// прерывает прогон: тема пропускается, результат собирается из удачных ответов.
// Нет ни одного ответа — текст пуст и возвращается последняя ошибка.
func (s *DeepScout) Run(ctx context.Context, topics []string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var answers []string
	var lastErr error
	for _, topic := range topics {
		topic = strings.TrimSpace(topic)
		if topic == "" {
			continue
		}
		if !s.takeBudget() {
			break
		}
		text, err := s.call(ctx, topic)
		if err != nil {
			lastErr = err
			continue
		}
		answers = append(answers, text)
	}
	if len(answers) == 0 {
		return "", lastErr
	}
	return strings.Join(answers, "\n\n"), nil
}

// takeBudget резервирует один вызов разведки; false — бюджет исчерпан.
func (s *DeepScout) takeBudget() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.spent >= s.cfg.Budget {
		return false
	}
	s.spent++
	return true
}

// call — один вызов разведки с тайм-аутом и обязательной записью в журналы.
func (s *DeepScout) call(ctx context.Context, topic string) (string, error) {
	started := time.Now()
	prompt := scoutPrompt(topic)
	callCtx, cancel := context.WithTimeout(ctx, s.cfg.Timeout)
	defer cancel()

	text, err := s.stream(callCtx, prompt)
	duration := time.Since(started)
	s.record(started, prompt, text, duration, err)
	s.logCall(topic, prompt, text, duration, err)
	if err != nil {
		return "", err
	}
	return text, nil
}

// stream читает поток слота до закрытия или тайм-аута.
func (s *DeepScout) stream(ctx context.Context, prompt string) (string, error) {
	if s.slot == nil {
		return "", ErrScoutNoSlot
	}
	ch, err := s.slot.Stream(ctx, prompt)
	if err != nil {
		// Клиент может вернуть отмену запроса из Do: тайм-аут разведки —
		// отдельная ошибка, по ней пропуск вызова не считается отказом API.
		if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
			return "", ErrScoutTimeout
		}
		return "", err
	}
	var b strings.Builder
	for {
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return b.String(), ErrScoutTimeout
			}
			return b.String(), ctx.Err()
		case chunk, ok := <-ch:
			if !ok {
				if strings.TrimSpace(b.String()) == "" {
					return "", ErrScoutNoAnswer
				}
				return b.String(), nil
			}
			b.WriteString(chunk)
		}
	}
}

// record пишет строку вызова в канонический журнал вызовов (FIX-4): роль,
// провайдер, оценка токенов, латентность, ошибка, остаток лимита, адрес, размер
// промпта и краткая сводка результата. Пишется и при отказе — «обязательное»
// логирование включает отказы.
func (s *DeepScout) record(started time.Time, prompt, text string, duration time.Duration, err error) {
	errText := ""
	if err != nil {
		errText = err.Error()
	}
	provider := s.cfg.Provider
	limit := ""
	if s.slot != nil {
		if name := strings.TrimSpace(s.slot.ProviderName()); name != "" {
			provider = name
		}
		limit = s.slot.LimitRemaining()
	}
	rec := models.CallRecord{
		TS:             started.UTC().Format(time.RFC3339),
		Role:           RoleScout,
		Provider:       provider,
		Calls:          1,
		Tokens:         estimateTokens(prompt) + estimateTokens(text),
		DurationMS:     duration.Milliseconds(),
		Error:          errText,
		LimitRemaining: limit,
		Endpoint:       s.cfg.Endpoint,
		PromptBytes:    int64(len(prompt)),
		Result:         summarizeResult(text),
	}
	_ = models.AppendCall(s.cfg.UsagePath, rec)
}

// logCall пишет событие вызова разведки в журнал станка: те же поля, что в
// журнале вызовов, плюс тема и модель провайдера.
func (s *DeepScout) logCall(topic, prompt, text string, duration time.Duration, err error) {
	if s.cfg.Logger == nil {
		return
	}
	status := "ok"
	errText := ""
	if err != nil {
		status = "error"
		errText = err.Error()
	}
	fields := []journal.Field{
		{Key: "topic", Value: topic},
		{Key: "provider", Value: s.cfg.Provider},
		{Key: "endpoint", Value: s.cfg.Endpoint},
		{Key: "prompt_bytes", Value: strconv.Itoa(len(prompt))},
		{Key: "tokens", Value: strconv.FormatInt(estimateTokens(prompt)+estimateTokens(text), 10)},
		{Key: "duration_ms", Value: strconv.FormatInt(duration.Milliseconds(), 10)},
		{Key: "status", Value: status},
	}
	if errText != "" {
		fields = append(fields, journal.Field{Key: "error", Value: errText})
	}
	summary := summarizeResult(text)
	if summary != "" {
		fields = append(fields, journal.Field{Key: "result", Value: summary})
	}
	_ = s.cfg.Logger.Event("paganel", journal.EventScoutCall, fields...)
}

// defaultScoutPrompt — дефолт промпта глубокой разведки (без файла пакета):
// короткая фактическая справка по теме мира, без художественного текста.
const defaultScoutPrompt = "Разведка мира. Собери короткую фактическую справку по теме: {topic}. Ответь тремя-пятью предложениями, без вступления и без вымысла."

// scoutPrompt — промпт одного вызова глубокой разведки: короткая фактическая
// справка по теме мира, без художественного текста.
func scoutPrompt(topic string) string {
	return prompts.Render(RoleScout, "scout", defaultScoutPrompt, map[string]any{"topic": topic})
}

// estimateTokens — оценка токенов по размеру текста: стриминг провайдера
// счётчика не отдаёт, поэтому ~4 символа на токен.
func estimateTokens(text string) int64 {
	n := len([]rune(strings.TrimSpace(text)))
	if n == 0 {
		return 0
	}
	return int64((n + 3) / 4)
}

// summarizeResult — краткая сводка ответа одной строкой для журнала.
func summarizeResult(text string) string {
	text = strings.Join(strings.Fields(text), " ")
	r := []rune(text)
	if len(r) <= ScoutResultSummaryLen {
		return text
	}
	return string(r[:ScoutResultSummaryLen])
}

// LoadDeepScout собирает живую глубокую разведку из конфигурации экземпляра по
// существующим конвенциям (B-14.4, B-4): config.yaml (секция paganel:
// переключатель, бюджет, тайм-аут, key_ref), providers.toml (адрес, протокол и
// заводская модель провайдера с именем key_ref) и settings.toml (ключ под тем же
// именем). Разведка выключена, провайдер или ключ не заданы — nil: пропуск
// разведки не блокирует рождение (B-14.3).
func LoadDeepScout(configPath, providersPath, settingsPath, usagePath string, logger *journal.Logger) *DeepScout {
	cfg, err := LoadPaganelConfig(configPath)
	if err != nil || !cfg.Scout.Enabled {
		return nil
	}
	name := strings.TrimSpace(cfg.Scout.KeyRef)
	if name == "" {
		name = "paganel"
	}
	def, ok := providercfg.Load(providersPath).Provider(name)
	if !ok {
		return nil
	}
	keys := loadSettingsKeys(settingsPath)
	key := strings.TrimSpace(keys[name])
	if key == "" {
		return nil
	}
	model := strings.TrimSpace(keys[name+"_model"])
	if model == "" {
		model = def.DefaultModel
	}
	if model == "" && def.Protocol == providercfg.ProtocolGemini {
		model = models.ModelFor(models.Provider(name))
	}
	slot, err := newScoutSlot(def, model, key)
	if err != nil {
		return nil
	}
	return NewDeepScout(DeepScoutConfig{
		Provider:  name,
		Endpoint:  def.BaseURL,
		Budget:    cfg.Scout.Budget,
		Timeout:   cfg.Scout.Timeout,
		UsagePath: usagePath,
		Logger:    logger,
	}, slot)
}

// newScoutSlot собирает слот по протоколу провайдера (те же клиенты, что у
// слотов Ткача): gemini — модельный клиент, openai — /chat/completions, local —
// запаска :5001.
func newScoutSlot(def providercfg.Provider, model, key string) (ScoutSlot, error) {
	switch def.Protocol {
	case providercfg.ProtocolGemini:
		slot, err := models.NewProviderClient(def.BaseURL, model, key)
		if err != nil {
			return nil, err
		}
		slot.SetProvider(def.Name)
		return slot, nil
	case providercfg.ProtocolOpenAI:
		slot, err := models.NewOpenAIClient(def.BaseURL, model, key)
		if err != nil {
			return nil, err
		}
		slot.SetProvider(def.Name)
		return slot, nil
	case providercfg.ProtocolLocal:
		slot, err := models.NewWeaver(models.DefaultWeaverURL)
		if err != nil {
			return nil, err
		}
		slot.SetProvider(def.Name)
		return slot, nil
	default:
		return nil, fmt.Errorf("paganel: протокол %q не поддерживает разведку", def.Protocol)
	}
}

// loadSettingsKeys читает settings.toml как top-level строки key = "value";
// файла нет — пустая карта.
func loadSettingsKeys(path string) map[string]string {
	out := map[string]string{}
	data, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		if unquoted, err := strconv.Unquote(val); err == nil {
			val = unquoted
		} else {
			val = strings.Trim(val, `"'`)
		}
		if key != "" {
			out[key] = val
		}
	}
	return out
}
