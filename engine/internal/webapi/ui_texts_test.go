package webapi

import (
	"io/fs"
	"strings"
	"testing"

	web "tns65/web"
)

// Интерфейсные тексты без сленга «станок» (T-13-09, UI-08): и словарь, и его
// зеркала в разметке. Игровые термины (рождение героя, сословие) не трогаются.
func TestUITextsWithoutStationSlang(t *testing.T) {
	names := []string{"static/i18n.js"}
	entries, err := fs.ReadDir(web.FS, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".html") {
			names = append(names, e.Name())
		}
	}

	for _, name := range names {
		data, err := web.FS.ReadFile(name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		low := strings.ToLower(string(data))
		for _, slang := range []string{"станк", "станци"} {
			if strings.Contains(low, slang) {
				t.Errorf("%s: интерфейсный сленг %q", name, slang)
			}
		}
	}
}

// Проверка станции в шаге 0 читается без цвета: знак ✓/✗ в той же строке рендера.
func TestWizardStationChecksUseGlyphs(t *testing.T) {
	data, err := web.FS.ReadFile("static/app.js")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.Contains(line, "wizard-check__dot") || !strings.Contains(line, "r.ok") {
			continue
		}
		found = true
		if !strings.Contains(line, "✓") || !strings.Contains(line, "✗") {
			t.Errorf("строка рендера проверки без знаков ✓/✗: %s", strings.TrimSpace(line))
		}
	}
	if !found {
		t.Fatal("в app.js нет строки рендера проверки станции")
	}
}

// Глиф вместо круга 10×10 px: у знака цвет только дублирует форму.
func TestWizardCheckDotHasNoCircle(t *testing.T) {
	data, err := web.FS.ReadFile("wizard.html")
	if err != nil {
		t.Fatal(err)
	}
	page := string(data)
	start := strings.Index(page, ".wizard-check__dot {")
	if start < 0 {
		t.Fatal("в wizard.html нет правила .wizard-check__dot")
	}
	end := strings.Index(page[start:], "}")
	if end < 0 {
		t.Fatal("правило .wizard-check__dot не закрыто")
	}
	rule := page[start : start+end]
	if strings.Contains(rule, "border-radius") || strings.Contains(rule, "background:") {
		t.Errorf("проверка станции всё ещё цветной кружок: %s", strings.Join(strings.Fields(rule), " "))
	}
}

// Формулировки понятны владельцу без сленга разработки.
func TestWizardTextsForOwner(t *testing.T) {
	data, err := web.FS.ReadFile("static/i18n.js")
	if err != nil {
		t.Fatal(err)
	}
	dict := string(data)
	for _, want := range []string{
		`"atlas.birth.submit": "Создать мир"`,
		`"wizard.world.title": "Создайте мир"`,
		`"wizard.step0": "Компьютер"`,
		`"wizard.station.title": "Проверяю компьютер"`,
		`"wizard.model.hint": "Две группы: основная модель пишет прозу, эмбеддинг понимает смысл. Установите хотя бы одну основную модель и один эмбеддинг — остальное настраивается позже в «Настройках»."`,
	} {
		if !strings.Contains(dict, want) {
			t.Errorf("i18n.js без строки %s", want)
		}
	}

	page, err := web.FS.ReadFile("wizard.html")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Проверяю компьютер", "Создать мир", "эмбеддинг понимает смысл"} {
		if !strings.Contains(string(page), want) {
			t.Errorf("wizard.html без зеркала %q", want)
		}
	}
}
