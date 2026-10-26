package sediment

// SedimentRecord — осевшая запись литературной базы (T-04-01, шаг 2).
// Ссылка на оригинал единственная — SourceSceneID; копии оригинала нет:
// исходная сцена остаётся в scenes.prose.
type SedimentRecord struct {
	SourceSceneID int64
	CleanedText   string
	Slots         []string
	Verdict       string
	TrustLevel    int
}
