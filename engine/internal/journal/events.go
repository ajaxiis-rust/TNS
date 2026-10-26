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
	EventTurnStage             = "turn.stage"
	EventSawPhaseBreak         = "saw.phase_break"
	EventTurnSawRecommendation = "turn.saw_recommendation"
	EventTurnSelectorDecision  = "turn.selector_decision"
	EventTurnPath              = "turn.path"
	EventTurnTrust             = "turn.trust"
	EventTurnLatency           = "turn.latency"
	EventTurnDegraded          = "turn.degraded"
	EventTurnFallback          = "turn.fallback"
)
