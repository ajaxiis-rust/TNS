package journal

// Константы имён событий журнала — единственный источник имён.
// Строковые литералы имён событий в коде запрещены (план T-01-03, шаг 2).
const (
	EventDBForeignWriter       = "db.foreign_writer"
	EventNetgateDeny           = "netgate.deny"
	EventPortgateDeny          = "portgate.deny"
	EventChildDegraded         = "child.degraded"
	EventChildOutput           = "child.output"
	EventChildStopped          = "child.stopped"
	EventLemmaFail             = "lemma.fail"
	EventSwarmVersionMismatch  = "swarm.version_mismatch"
	EventWebAccess             = "web.access"
	EventEmbedFail             = "embed.fail"
	EventAnchorForced          = "birth.anchor_forced"
	EventBaseCorrupt           = "birth.base_corrupt"
	EventTurnStage             = "turn.stage"
	EventSawPhaseBreak         = "saw.phase_break"
	EventSlotDegraded          = "slot.degraded"
	EventSlotWiring            = "slot.wiring_fail"
	EventTurnSawRecommendation = "turn.saw_recommendation"
	EventTurnSelectorDecision  = "turn.selector_decision"
	EventTurnPath              = "turn.path"
	EventTurnTrust             = "turn.trust"
	EventTurnLatency           = "turn.latency"
	EventTurnDegraded          = "turn.degraded"
	EventTurnFallback          = "turn.fallback"
	EventTurnComposition       = "turn.composition"
	EventDeepreadCall          = "deepread.call"
	// Слот Редактора и Цензор правок (FIX-5): вызов, принятый ответ и отказ
	// модельного шага видны в журнале станка.
	EventEditorDigestCall   = "editor.digest.call"
	EventEditorDigestReply  = "editor.digest.reply"
	EventEditorDigestRefuse = "editor.digest.refuse"
	EventEditorCensorReject = "editor.censor_reject"
	// Разведка Паганеля (DEC-4, B-14.4): каждый вызов глубокой разведки и
	// оседание её отчёта видны в журнале станка для отладки.
	EventScoutCall   = "scout.call"
	EventScoutReport = "scout.report"
)

// Имена полей событий, общих для нескольких пакетов.
const (
	// FieldRegistryChanged — дрейф реестра семей в событии turn.composition
	// (FIX-6): текущий реестр разошёлся с отпечатком рождения мира.
	FieldRegistryChanged = "registry_changed"
)
