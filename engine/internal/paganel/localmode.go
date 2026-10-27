package paganel

import "tns65/internal/workshop"

// LocalMode — режим «только локально» (T-06-11): страховка, запрещающая внешние
// вызовы Паганеля. Действует при пресете local из реестра T-02-11 (дефолт);
// внешний слот выбирается только явно (F-07). В выдуманном мире наружу не
// выходим по построению (W-07.5), поэтому внешний ключ Паганеля живому контуру
// недоступен: ключ остаётся в settings.toml, здесь хранится только ссылка
// key_ref (F-03.4).
type LocalMode struct {
	preset     workshop.Provider
	scoutCalls int
	forgeCalls int
}

// NewLocalMode создаёт режим по пресету слота (реестр T-02-11). Пустой пресет
// трактуется как локальный (дефолт реестра, F-07).
func NewLocalMode(preset workshop.Provider) *LocalMode {
	if preset == "" {
		preset = workshop.DefaultProvider
	}
	return &LocalMode{preset: preset}
}

// Preset возвращает пресет режима.
func (m *LocalMode) Preset() workshop.Provider { return m.preset }

// IsLocal сообщает, действует ли локальный пресет: внешних вызовов нет.
func (m *LocalMode) IsLocal() bool { return m.preset == workshop.ProviderLocal }

// ScoutEnabled — эффективное разрешение «походов наружу» (F-08): при локальном
// пресете переключатель игнорируется — наружу не уходит ничего (шаг 2).
func (m *LocalMode) ScoutEnabled(switchEnabled bool) bool {
	if m.IsLocal() {
		return false
	}
	return switchEnabled
}

// ScoutConfig возвращает настройки разведки, приведённые к режиму: при локальном
// пресете Enabled=false независимо от переключателя.
func (m *LocalMode) ScoutConfig(cfg ScoutConfig) ScoutConfig {
	cfg.Enabled = m.ScoutEnabled(cfg.Enabled)
	return cfg
}

// ForgeIsLocal сообщает, идёт ли ковка Паганеля на локальной модели: при
// локальном пресете — всегда локально, внешних вызовов ковки нет (шаг 1).
func (m *LocalMode) ForgeIsLocal() bool { return m.IsLocal() }

// ScoutCalls — счётчик внешних вызовов разведки. При локальном пресете всегда 0.
func (m *LocalMode) ScoutCalls() int { return m.scoutCalls }

// ForgeCalls — счётчик внешних вызовов ковки. При локальном пресете всегда 0.
func (m *LocalMode) ForgeCalls() int { return m.forgeCalls }

// ScoutGate — разрешение на внешний вызов разведки: локальный пресет или
// выключенный переключатель — отказ без учёта вызова; разрешённый внешний вызов
// учитывается в счётчике ScoutCalls.
func (m *LocalMode) ScoutGate(switchEnabled bool) bool {
	if !m.ScoutEnabled(switchEnabled) {
		return false
	}
	m.scoutCalls++
	return true
}

// ForgeGate — разрешение на внешний вызов ковки: локальный пресет — всегда отказ
// (ковка на локальной модели); разрешённый внешний вызов учитывается в счётчике
// ForgeCalls.
func (m *LocalMode) ForgeGate() bool {
	if m.ForgeIsLocal() {
		return false
	}
	m.forgeCalls++
	return true
}
