package sediment

// Source — источник событий журнала оседания (T-04-09, шаг 1).
const Source = "sediment"

// Имена событий журнала оседания — единственный источник имён (T-04-09, шаг 1).
// Строковые литералы имён событий в коде запрещены (T-01-03, шаг 2).
const (
	EventIn                = "sediment.in"
	EventPrefiltered       = "sediment.prefiltered"
	EventClean             = "sediment.clean"
	EventAutocriticVerdict = "sediment.autocritic.verdict"
	EventDedup             = "sediment.dedup"
	EventDupSkip           = "sediment.dup_skip"
	EventCensorReject      = "sediment.censor_reject"
	EventWrite             = "sediment.write"
)
