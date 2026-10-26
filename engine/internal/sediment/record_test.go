package sediment_test

import (
	"errors"
	"reflect"
	"testing"

	"tns65/internal/sediment"
)

func TestSlotSetClosed(t *testing.T) {
	want := []sediment.Token{
		sediment.TokenName,
		sediment.TokenThing,
		sediment.TokenPrice,
		sediment.TokenPlace,
	}

	got := sediment.SlotTokens()
	if len(got) != 4 {
		t.Fatalf("слот-токенов в наборе: %d, want 4", len(got))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("слот-токен %d = %q, want %q", i, got[i], want[i])
		}
	}

	for _, tk := range want {
		parsed, err := sediment.ParseToken(string(tk))
		if err != nil {
			t.Errorf("токен %q из набора отвергнут: %v", tk, err)
			continue
		}
		if parsed != tk {
			t.Errorf("ParseToken(%q) = %q, want %q", tk, parsed, tk)
		}
		if sediment.BinderToken(tk) == "" {
			t.Errorf("нет соответствия биндера для токена %q", tk)
		}
	}

	if _, err := sediment.ParseToken("{FOO}"); err == nil {
		t.Errorf("токен {FOO} вне набора, но принят")
	} else if !errors.Is(err, sediment.ErrUnknownSlotToken) {
		t.Errorf("ParseToken({FOO}) вернул не ErrUnknownSlotToken: %v", err)
	}
}

func TestRecordLinksOriginal(t *testing.T) {
	const sourceSceneID int64 = 42

	rec := sediment.SedimentRecord{
		SourceSceneID: sourceSceneID,
		CleanedText:   "{NAME} вошёл в {PLACE}.",
		Slots:         []string{string(sediment.TokenName), string(sediment.TokenPlace)},
		Verdict:       "clean",
		TrustLevel:    1,
	}

	if rec.SourceSceneID != sourceSceneID {
		t.Fatalf("SourceSceneID = %d, want %d", rec.SourceSceneID, sourceSceneID)
	}

	// Запись не хранит копию оригинала: поля ровно контрактные, поля для исходной
	// прозы нет — ссылка на сцену только через SourceSceneID.
	typ := reflect.TypeOf(sediment.SedimentRecord{})
	wantFields := map[string]bool{
		"SourceSceneID": true,
		"CleanedText":   true,
		"Slots":         true,
		"Verdict":       true,
		"TrustLevel":    true,
	}
	if typ.NumField() != len(wantFields) {
		t.Fatalf("число полей SedimentRecord = %d, want %d", typ.NumField(), len(wantFields))
	}
	for i := 0; i < typ.NumField(); i++ {
		name := typ.Field(i).Name
		if !wantFields[name] {
			t.Errorf("поле вне контракта SedimentRecord: %s", name)
		}
	}
}
