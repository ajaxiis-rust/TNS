package birth

import (
	"reflect"
	"testing"
)

func TestPsychotypeSpace(t *testing.T) {
	// осей 4
	if len(Axes) != 4 {
		t.Fatalf("осей: %d, want 4", len(Axes))
	}
	wantAxes := []string{"E/I", "N/S", "T/F", "J/P"}
	for i, ax := range Axes {
		if ax.Name != wantAxes[i] {
			t.Fatalf("ось %d: %q, want %q", i, ax.Name, wantAxes[i])
		}
	}

	// метрик 14, каждая привязана к известной оси
	metrics, err := LoadMetrics()
	if err != nil {
		t.Fatalf("LoadMetrics: %v", err)
	}
	if len(metrics) != 14 {
		t.Fatalf("метрик: %d, want 14", len(metrics))
	}
	seen := map[string]bool{}
	for _, m := range metrics {
		if m.Name == "" {
			t.Fatal("метрика без имени")
		}
		if seen[m.Name] {
			t.Fatalf("метрика %q повторяется", m.Name)
		}
		seen[m.Name] = true
		if _, ok := axisIndex(m.Axis); !ok {
			t.Fatalf("метрика %q ссылается на неизвестную ось %q", m.Name, m.Axis)
		}
	}

	// кодов 16: пространство комбинаций полюсов, все уникальны
	codes := AllCodes()
	if len(codes) != 16 {
		t.Fatalf("кодов: %d, want 16", len(codes))
	}
	codeSet := map[string]bool{}
	for _, c := range codes {
		s := c.String()
		if codeSet[s] {
			t.Fatalf("код %q повторяется в пространстве", s)
		}
		codeSet[s] = true
	}
	if len(codeSet) != 16 {
		t.Fatalf("уникальных кодов: %d, want 16", len(codeSet))
	}

	// профиль из кода: 14 метрик, имена совпадают с metrics.json
	code, err := ParseCode("ENTJ")
	if err != nil {
		t.Fatalf("ParseCode: %v", err)
	}
	profile, err := ProfileFromCode(code, metrics)
	if err != nil {
		t.Fatalf("ProfileFromCode: %v", err)
	}
	if len(profile) != 14 {
		t.Fatalf("метрик профиля: %d, want 14", len(profile))
	}
	for _, m := range metrics {
		if _, ok := profile[m.Name]; !ok {
			t.Fatalf("метрика %q отсутствует в профиле", m.Name)
		}
	}

	// повторный вызов не меняет профиль: детерминированно, без rand и без LLM
	again, err := ProfileFromCode(code, metrics)
	if err != nil {
		t.Fatalf("ProfileFromCode (повтор): %v", err)
	}
	if !reflect.DeepEqual(profile, again) {
		t.Fatalf("профиль изменился при повторном вызове: %v != %v", profile, again)
	}
}

func TestJungArchetypes(t *testing.T) {
	archs, err := LoadArchetypes()
	if err != nil {
		t.Fatalf("LoadArchetypes: %v", err)
	}
	if len(archs) != 12 {
		t.Fatalf("архетипов: %d, want 12", len(archs))
	}

	// отдельный список для лиц мира: не сводится к 16 кодам психотипа
	if len(archs) == len(AllCodes()) {
		t.Fatal("архетипы и коды психотипа совпали по числу — списки не разделены")
	}

	seen := map[string]bool{}
	for _, a := range archs {
		if a == "" {
			t.Fatal("пустое имя архетипа")
		}
		if seen[a] {
			t.Fatalf("архетип %q повторяется", a)
		}
		seen[a] = true
	}
}
