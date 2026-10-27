package book

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"os/exec"
	"strings"
	"testing"

	"tns65/internal/db"
	"tns65/internal/testkit"
)

// claimText — действующая заявка игрока (1–3 предложения).
const claimText = "Герой ищет дорогу домой. Путь его лежит через кольцо гор."

// proposalText — предложение модели (1–3 предложения) для слота заявки.
const proposalText = "Герой идёт к дому. В пути его ждут испытания."

// newClaimWorld создаёт оперативную базу мира с синопсисом анкеты в world_meta
// (пишет forge по карте писателей) и возвращает хэндл core.
func newClaimWorld(t *testing.T, synopsis string) *db.Handle {
	t.Helper()
	path := t.TempDir() + "/world(a).db"

	w, err := db.OpenWriter(path)
	if err != nil {
		t.Fatalf("OpenWriter: %v", err)
	}
	if err := db.Migrate(w, "world", "shared"); err != nil {
		_ = w.Close()
		t.Fatalf("Migrate(world, shared): %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	fh, err := db.OpenWriterAs(path, db.DBWorld, db.WriterForge, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs(forge): %v", err)
	}
	raw, err := json.Marshal(synopsis)
	if err != nil {
		_ = fh.Close()
		t.Fatalf("json.Marshal(synopsis): %v", err)
	}
	if _, err := fh.Exec("world_meta",
		`INSERT INTO world_meta ("key", value) VALUES ('synopsis', ?)`, string(raw)); err != nil {
		_ = fh.Close()
		t.Fatalf("INSERT world_meta synopsis: %v", err)
	}
	if err := fh.Close(); err != nil {
		t.Fatalf("Close(forge): %v", err)
	}

	h, err := db.OpenWriterAs(path, db.DBWorld, db.WriterCore, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs(world): %v", err)
	}
	t.Cleanup(func() { _ = h.Close() })
	return h
}

// insertAtlasPattern пишет строку карты игрока (player_atlas) хэндлом core.
func insertAtlasPattern(t *testing.T, h *db.Handle, theme, value string) {
	t.Helper()
	if _, err := h.Exec("player_atlas",
		`INSERT INTO player_atlas (day, theme, mood, interest, source) VALUES (1, ?, 'спокойный', ?, 'pythia')`,
		theme, value); err != nil {
		t.Fatalf("INSERT player_atlas: %v", err)
	}
}

// TestClaim — «Проверка» T-07-09.
func TestClaim(t *testing.T) {
	t.Run("книга создаётся и играется без заявки", func(t *testing.T) {
		world := newBookWorld(t)

		if _, err := CurrentClaim(world); !errors.Is(err, ErrNoClaim) {
			t.Fatalf("CurrentClaim без заявки = %v, want ErrNoClaim", err)
		}

		// Публикация главы без заявки работает — книга без заявки функционирует.
		ch := seedReadyChapter(t, world)
		if err := Process(context.Background(), world, testkit.NewFakeEmbed(), ch.ID); err != nil {
			t.Fatalf("Process без заявки: %v", err)
		}

		var claims int
		if err := world.QueryRow(`SELECT COUNT(*) FROM book_claim`).Scan(&claims); err != nil {
			t.Fatalf("COUNT(book_claim): %v", err)
		}
		if claims != 0 {
			t.Fatalf("строк book_claim без заявки = %d, want 0", claims)
		}
	})

	t.Run("заявка хранит текст и вектор dim=1024", func(t *testing.T) {
		world := newBookWorld(t)
		embed := testkit.NewFakeEmbed()

		claim, err := SaveClaim(context.Background(), world, embed, claimText, ClaimApproved)
		if err != nil {
			t.Fatalf("SaveClaim: %v", err)
		}
		if claim.ID <= 0 || claim.VectorID <= 0 {
			t.Fatalf("SaveClaim = %+v, want заполненные id и vector_id", claim)
		}

		var text, status string
		var vectorID int64
		if err := world.QueryRow(`SELECT text, vector_id, status FROM book_claim WHERE id = ?`, claim.ID).
			Scan(&text, &vectorID, &status); err != nil {
			t.Fatalf("SELECT book_claim: %v", err)
		}
		if text != claimText {
			t.Fatalf("text заявки = %q, want %q", text, claimText)
		}
		if status != string(ClaimApproved) {
			t.Fatalf("status заявки = %q, want approved", status)
		}
		if vectorID != claim.VectorID {
			t.Fatalf("vector_id = %d, want %d", vectorID, claim.VectorID)
		}

		var ownerTable, sourceText string
		var dim int
		var blob []byte
		if err := world.QueryRow(
			`SELECT owner_table, dim, source_text, vec FROM vectors WHERE owner_table = 'book_claim' AND owner_id = ?`,
			claim.ID).Scan(&ownerTable, &dim, &sourceText, &blob); err != nil {
			t.Fatalf("SELECT vectors: %v", err)
		}
		if ownerTable != "book_claim" {
			t.Fatalf("owner_table = %q, want book_claim", ownerTable)
		}
		if dim != 1024 {
			t.Fatalf("dim = %d, want 1024", dim)
		}
		if sourceText != claimText {
			t.Fatalf("source_text вектора = %q, want %q", sourceText, claimText)
		}
		wantVec, err := embed.Embed(context.Background(), claimText)
		if err != nil {
			t.Fatalf("Embed: %v", err)
		}
		gotVec := decodeVec(blob)
		if len(gotVec) != 1024 {
			t.Fatalf("длина вектора заявки = %d, want 1024", len(gotVec))
		}
		for i := range gotVec {
			if gotVec[i] != wantVec[i] {
				t.Fatalf("vec[%d] = %v, want %v", i, gotVec[i], wantVec[i])
			}
		}
	})

	t.Run("дрейф вычислен и отличен от нуля", func(t *testing.T) {
		world := newBookWorld(t)
		embed := testkit.NewFakeEmbed()

		claim, err := SaveClaim(context.Background(), world, embed, claimText, ClaimApproved)
		if err != nil {
			t.Fatalf("SaveClaim: %v", err)
		}

		ch := seedReadyChapter(t, world)
		if err := Process(context.Background(), world, embed, ch.ID); err != nil {
			t.Fatalf("Process: %v", err)
		}

		drift, err := ClaimDrift(world, claim.ID)
		if err != nil {
			t.Fatalf("ClaimDrift: %v", err)
		}
		if math.IsNaN(drift) {
			t.Fatal("дрейф = NaN")
		}
		if drift == 0 {
			t.Fatal("дрейф = 0, want отличен от нуля")
		}
		if drift < 0 {
			t.Fatalf("дрейф = %v, want >= 0 (cos ≤ 1)", drift)
		}
	})

	t.Run("предложение формируется после трёх сессий", func(t *testing.T) {
		world := newClaimWorld(t, "Герой ищет дорогу домой.")
		embed := testkit.NewFakeEmbed()
		slot := testkit.NewFakeSlot()
		slot.SetResponse(ClaimProposalRole, proposalText)

		for i := 0; i < 2; i++ {
			insertOpenSession(t, world)
		}
		if _, err := ProposeClaim(context.Background(), world, slot, embed); !errors.Is(err, ErrClaimTooFewSessions) {
			t.Fatalf("ProposeClaim(2 сессии) = %v, want ErrClaimTooFewSessions", err)
		}

		insertAtlasPattern(t, world, "interest", "дорога")
		insertAtlasPattern(t, world, "habit", "ходить пешком")
		insertOpenSession(t, world) // третья сессия.

		claim, err := ProposeClaim(context.Background(), world, slot, embed)
		if err != nil {
			t.Fatalf("ProposeClaim(3 сессии): %v", err)
		}
		if claim.Status != ClaimProposed {
			t.Fatalf("status предложения = %q, want proposed", claim.Status)
		}
		if claim.Text != proposalText {
			t.Fatalf("текст предложения = %q, want %q", claim.Text, proposalText)
		}
		if slot.Calls() != 1 {
			t.Fatalf("вызовов слота = %d, want 1", slot.Calls())
		}
	})

	t.Run("selector не импортирует book", func(t *testing.T) {
		out, err := exec.Command("go", "list", "-deps", "tns65/internal/selector").CombinedOutput()
		if err != nil {
			t.Fatalf("go list -deps tns65/internal/selector: %v", err)
		}
		for _, line := range strings.Split(string(out), "\n") {
			if strings.TrimSpace(line) == "tns65/internal/book" {
				t.Fatalf("selector (или saw) импортирует пакет book: %s", line)
			}
		}
	})
}
