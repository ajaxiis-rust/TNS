// Package deepread — первичный разбор книги (W-22): словари категорий
// языкового пакета и реестр ключей разбора.
package deepread

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// minLemmaWeight, maxLemmaWeight — границы веса леммы словаря (W-22.1).
const (
	minLemmaWeight = 1
	maxLemmaWeight = 3
)

// Ключи категорий разбора (W-22.1, §3 плана волны 14). Порядок фиксирован:
// индекс ключа в реестре CategoryKeys служит owner_id якоря dict_anchor.
const (
	KeyAppearance     = "appearance"
	KeyClothing       = "clothing"
	KeyNature         = "nature"
	KeyArchitecture   = "architecture"
	KeyEmotion        = "emotion"
	KeyActionDramatic = "action_dramatic"
	KeySpeech         = "speech"
	KeyTime           = "time"
	KeySocial         = "social"
	KeyErotic         = "erotic"
	KeyViolence       = "violence"
	KeyBattle         = "battle"
)

// CategoryKeys — реестр ключей категорий разбора: перечень из §3 плана волны
// 14. clothing, emotion и time берутся из существующих файлов пака (W-22.2),
// остальные — из собственных файлов словарей (appearance — с IMP-9).
var CategoryKeys = []string{
	KeyAppearance,
	KeyClothing,
	KeyNature,
	KeyArchitecture,
	KeyEmotion,
	KeyActionDramatic,
	KeySpeech,
	KeyTime,
	KeySocial,
	KeyErotic,
	KeyViolence,
	KeyBattle,
}

// DictionaryKeys — ключи категорий, у которых есть собственный файл словаря в
// пакете (W-22.3); порядок совпадает с перечнем файлов волны.
var DictionaryKeys = []string{
	KeyNature,
	KeyArchitecture,
	KeySocial,
	KeyActionDramatic,
	KeyErotic,
	KeyViolence,
	KeyBattle,
	KeyAppearance,
}

// dictFiles — имя файла пака для ключа категории (W-22.2: clothing и character
// — оси descriptions.json, emotion — detectors_mood.json, time —
// detectors_time.json; параллельные словари запрещены). appearance с IMP-9
// имеет собственный словарь анатомии (решение владельца 15.11).
var dictFiles = map[string]string{
	KeyNature:         "nature.json",
	KeyArchitecture:   "architecture.json",
	KeySocial:         "social.json",
	KeyActionDramatic: "action_dramatic.json",
	KeyErotic:         "erotic.json",
	KeyViolence:       "violence.json",
	KeyBattle:         "battle.json",
	KeyAppearance:     "appearance.json",
}

// Канон ярлыков стиля книги (W-27.2): ярлык вне этого списка не присваивается.
const (
	StyleDescriptive = "описательный"
	StyleDynamic     = "динамичный"
	StyleDialogic    = "диалоговый"
	StyleLyrical     = "лирический"
	StyleDry         = "сухой"
)

// StyleLabels — канон ярлыков стиля первого набора (W-27.2).
var StyleLabels = []string{
	StyleDescriptive,
	StyleDynamic,
	StyleDialogic,
	StyleLyrical,
	StyleDry,
}

// Признаки стилевого профиля книги (W-27.1): имена ключей порогов styles.json.
const (
	FeatureAvgSentenceLen   = "avg_sentence_len"
	FeatureDialogueShare    = "dialogue_share"
	FeatureExclaimDensity   = "exclaim_density"
	FeatureQuestionDensity  = "question_density"
	FeatureEllipsisDensity  = "ellipsis_density"
	FeatureLexicalDiversity = "lexical_diversity"
)

// CategoryID возвращает owner_id ключа категории — индекс в реестре
// CategoryKeys; ключ вне реестра — -1.
func CategoryID(key string) int {
	for i, k := range CategoryKeys {
		if k == key {
			return i
		}
	}
	return -1
}

// Phrase — композиция лемм словаря. Формат зафиксирован (W-22.1, §3), второй
// шаг волны поле не заполняет.
type Phrase struct {
	Lemmas []string `json:"lemmas"`
	Weight int      `json:"weight"`
}

// Dictionary — словарь категории: леммы с весом и композиции (W-22.1).
type Dictionary struct {
	Key     string
	Lemmas  map[string]int
	Phrases []Phrase
}

// dictFile — формат файла словаря категории (§3).
type dictFile struct {
	Lemmas  map[string]int `json:"lemmas"`
	Phrases []Phrase       `json:"phrases"`
}

// LoadDictionaries читает восемь словарей категорий из каталога пака. Отсутствие
// файла или пустой словарь — ошибка: тихий пустой словарь выключил бы разбор
// категории.
func LoadDictionaries(packDir string) (map[string]*Dictionary, error) {
	out := make(map[string]*Dictionary, len(DictionaryKeys))
	for _, key := range DictionaryKeys {
		d, err := LoadDictionary(packDir, key)
		if err != nil {
			return nil, err
		}
		out[key] = d
	}
	return out, nil
}

// LoadDictionary читает словарь одной категории из каталога пака. Леммы
// нормализуются в нижний регистр, вес обязан лежать в 1–3 (W-22.1).
func LoadDictionary(packDir, key string) (*Dictionary, error) {
	name, ok := dictFiles[key]
	if !ok {
		return nil, fmt.Errorf("deepread: у ключа %q нет файла словаря", key)
	}
	raw, err := os.ReadFile(filepath.Join(packDir, filepath.FromSlash(name)))
	if err != nil {
		return nil, err
	}
	var f dictFile
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("deepread: %s: %w", name, err)
	}
	if len(f.Lemmas) == 0 {
		return nil, fmt.Errorf("deepread: %s: словарь пуст", name)
	}

	d := &Dictionary{Key: key, Lemmas: make(map[string]int, len(f.Lemmas)), Phrases: f.Phrases}
	for lemma, weight := range f.Lemmas {
		lemma = strings.ToLower(strings.TrimSpace(lemma))
		if lemma == "" {
			return nil, fmt.Errorf("deepread: %s: пустая лемма", name)
		}
		if weight < minLemmaWeight || weight > maxLemmaWeight {
			return nil, fmt.Errorf("deepread: %s: лемма %q: вес %d вне %d..%d",
				name, lemma, weight, minLemmaWeight, maxLemmaWeight)
		}
		if _, dup := d.Lemmas[lemma]; dup {
			return nil, fmt.Errorf("deepread: %s: лемма %q повторяется", name, lemma)
		}
		d.Lemmas[lemma] = weight
	}
	return d, nil
}

// Styles — канон ярлыков стиля книги (W-27.2): фиксированный список ярлыков и
// пороги признаков; ярлык вне канона не присваивается.
type Styles struct {
	Version  int          `json:"version"`
	Features []string     `json:"features"`
	Labels   []StyleLabel `json:"labels"`
}

// StyleLabel — ярлык стиля и его пороги: признак → минимум и/или максимум.
type StyleLabel struct {
	Name string             `json:"name"`
	Min  map[string]float64 `json:"min,omitempty"`
	Max  map[string]float64 `json:"max,omitempty"`
}

// LoadStyles читает styles.json пака и проверяет канон: ярлыки не пусты и
// уникальны, пороги ссылаются на признаки из списка features (W-27.2).
func LoadStyles(packDir string) (*Styles, error) {
	raw, err := os.ReadFile(filepath.Join(packDir, "styles.json"))
	if err != nil {
		return nil, err
	}
	var s Styles
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("deepread: styles.json: %w", err)
	}
	if len(s.Labels) == 0 {
		return nil, fmt.Errorf("deepread: styles.json: канон ярлыков пуст")
	}
	features := make(map[string]struct{}, len(s.Features))
	for _, f := range s.Features {
		features[f] = struct{}{}
	}
	seen := make(map[string]struct{}, len(s.Labels))
	for _, l := range s.Labels {
		name := strings.TrimSpace(l.Name)
		if name == "" {
			return nil, fmt.Errorf("deepread: styles.json: ярлык без имени")
		}
		if _, dup := seen[name]; dup {
			return nil, fmt.Errorf("deepread: styles.json: ярлык %q повторяется", name)
		}
		seen[name] = struct{}{}
		for feature := range l.Min {
			if _, ok := features[feature]; !ok {
				return nil, fmt.Errorf("deepread: styles.json: ярлык %q: признак %q вне списка features", name, feature)
			}
		}
		for feature, max := range l.Max {
			min, ok := l.Min[feature]
			if _, okF := features[feature]; !okF {
				return nil, fmt.Errorf("deepread: styles.json: ярлык %q: признак %q вне списка features", name, feature)
			}
			if ok && min > max {
				return nil, fmt.Errorf("deepread: styles.json: ярлык %q: признак %q: min %g > max %g", name, feature, min, max)
			}
		}
	}
	return &s, nil
}
