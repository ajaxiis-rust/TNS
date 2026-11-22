# API Surface

Exported symbols only. Bodies stripped.

## `_/home/ajaxii/PRJ/TNS65/engine/internal/bake`

```go
func Bake(cfg Config) (error)
func TestBakeFreezes(t *testing.T)
func TestBakeIdempotent(t *testing.T)
struct Config
var ErrAlreadyBaked
var ErrNoNarratives
```

## `_/home/ajaxii/PRJ/TNS65/engine/internal/bank`

```go
const Budget
func (*Bank) Pick(tags []string) (Record, error)
func (mapBinder) Bind(text string) (string)
func New(reader *db.Handle, clock Clock, slot Slot, binder Binder) (*Bank)
func TestBank(t *testing.T)
interface Binder
interface Clock
interface Slot
struct Bank
struct Record
var ErrNoReader
var ErrNoRecord
```

## `_/home/ajaxii/PRJ/TNS65/engine/internal/birth`

```go
const ExitDuplicate
const ExitInvalid
func (*ExitError) Error() (string)
func (*ExitError) Unwrap() (error)
func (*ValidationError) Error() (string)
func (*countingEmbedder) Calls() (int)
func (*countingEmbedder) Embed(ctx context.Context, text string) ([]float32, error)
func (*countingScout) Run(context.Context, []string) (string, error)
func (*liveScoutSlot) LimitRemaining() (string)
func (*liveScoutSlot) ProviderName() (string)
func (*liveScoutSlot) Stream(ctx context.Context, prompt string) (chan string, error)
func (Code) String() (string)
func (Layers) PlayerJSON() ([]byte, error)
func (Layers) PlayerView() (PlayerView)
func AllCodes() ([]Code)
func AvailableLangs() ([]string, error)
func Birth(a *Ancketa, cfg Config) (*Result, error)
func CheckConsumers(c Consumers, fields []string) (error)
func DefaultMemoryParams() (MemoryParams)
func List(root string) ([]World, error)
func LoadAppearances() ([]string, error)
func LoadArchetypes() ([]string, error)
func LoadConsumers() (Consumers, error)
func LoadData() (Data, error)
func LoadEstates() ([]Estate, error)
func LoadMetrics() ([]Metric, error)
func LoadVoices() ([]string, error)
func NewLayers(psychotype string, mem MemoryParams) (Layers)
func Pack(a *Ancketa) ([]Card, error)
func ParseCode(s string) (Code, error)
func ProfileFromCode(code Code, metrics []Metric) (Profile, error)
func TestAnchorBench(t *testing.T)
func TestAnchorForced(t *testing.T)
func TestAnchorLangFilter(t *testing.T)
func TestAnchorMaterial(t *testing.T)
func TestAnchorMaterialMissingNarrative(t *testing.T)
func TestAnchorNearest(t *testing.T)
func TestAnchorNoLLM(t *testing.T)
func TestAncketa(t *testing.T)
func TestAncketaData(t *testing.T)
func TestAncketaRejectNoFiles(t *testing.T)
func TestAncketaSourceBook(t *testing.T)
func TestAncketaSynopsisSentences(t *testing.T)
func TestAncketaWorldRules(t *testing.T)
func TestAvailableLangs(t *testing.T)
func TestBirthBaseRootDefault(t *testing.T)
func TestBirthCreatesWorld(t *testing.T)
func TestBirthDescriptionTokens(t *testing.T)
func TestBirthDuplicateSlug(t *testing.T)
func TestBirthLocalPreset(t *testing.T)
func TestBirthOnShippedBases(t *testing.T)
func TestBirthRollback(t *testing.T)
func TestBirthSourceBookBinding(t *testing.T)
func TestBirthSourceBookRequired(t *testing.T)
func TestConsumers(t *testing.T)
func TestDeepScoutLiveAtBirth(t *testing.T)
func TestDeepScoutNoDuplicate(t *testing.T)
func TestDeepScoutUnavailableAtBirth(t *testing.T)
func TestEmptySynopsis(t *testing.T)
func TestEstatesCoverAllOrders(t *testing.T)
func TestGeoSeedFiction(t *testing.T)
func TestGeoSeedMixed(t *testing.T)
func TestGeoSeedReal(t *testing.T)
func TestIsekaiReplay(t *testing.T)
func TestJungArchetypes(t *testing.T)
func TestNoProseAtBirth(t *testing.T)
func TestNoProseAtBirthRejectsSeededProse(t *testing.T)
func TestNpcLayers(t *testing.T)
func TestPack96(t *testing.T)
func TestPackDeterministic(t *testing.T)
func TestPsychotypeSpace(t *testing.T)
func TestSeedFill(t *testing.T)
func TestValidateLang(t *testing.T)
func TestValidateLangExplicitPath(t *testing.T)
func TestValidateLangFields(t *testing.T)
func TestVerifyBaseChecksum(t *testing.T)
func TestWorldList(t *testing.T)
func Validate(a *Ancketa) (error)
func ValidateLang(a *Ancketa) (*FieldError)
func VerifyBaseChecksum(dbPath string) (error)
interface DeepScout
struct Ancketa
struct Axis
struct Card
struct Config
struct Consumers
struct Data
struct EconomyLayer
struct Estate
struct ExitError
struct FieldError
struct HiddenLayer
struct Humours
struct Layers
struct MemoryParams
struct Metric
struct PlayerView
struct Result
struct StateLayer
struct ValidationError
struct World
struct WorldFields
type Code
type Profile
var Axes
var ErrDuplicateSlug
var ErrNoWorld
var Fields
var WorldFrozenFields
var WorldReaskedFields
```

## `_/home/ajaxii/PRJ/TNS65/engine/internal/book`

```go
const AssembleVersion
const AutoCloseAfter
const AutoCloseEventKind
const BookWindowID
const ClaimApproved
const ClaimMaxSentences
const ClaimMinSentences
const ClaimMinSessions
const ClaimNightForgeBonus
const ClaimProposalRole
const ClaimProposed
const CloseByLimit
const CloseByPlayer
const DeepReadRole
const DigestMaxSentences
const DigestMinSentences
const DigestNone
const DigestShown
const EntryBookButton
const EntryHistoryTab
const FormatMarkdown
const KindChapter
const KindEpilogue
const PeakChapter
const PeakScene
const SleepIntent
const StatusClosed
const StatusOpen
const StatusProcessing
const StatusPublished
const UnconsciousIntent
const WakeForging
const WakeReady
const WorldNotesRole
func (*Lexicon) IntentLemmas(intent string) ([]string)
func (*Lexicon) SleepLemmas() ([]string)
func (*digestSlot) Generate(ctx context.Context, role, prompt string) (string, error)
func (*fakePauseClock) Now() (time.Time)
func (*notesSlot) Generate(_ context.Context, role, _ string) (string, error)
func (Chapter) Close(h *db.Handle, now string) (error)
func (Chapter) Text(h *db.Handle) (string, error)
func (Trigger) Tabs() ([]WindowTab)
func (Trigger) WindowID() (string)
func (editsLemmatizer) Lemmatize(text string) ([]lemmarpc.Token, error)
func (sleepLemmatizer) Lemmatize(text string) ([]lemmarpc.Token, error)
func ApplyPlayerEdits(h *db.Handle, chapterID int64, edits []PlayerEdit) (EditResult, error)
func ApproveClaim(h *db.Handle, claimID int64) (error)
func AssembleChapter(h *db.Handle, ch Chapter) (error)
func AutoClose(h *db.Handle, now string, slot Slot) (Chapter, bool, error)
func BuildDigest(ctx context.Context, h *db.Handle, slot DigestSlot, chapterID int64) (error)
func CanPublish(h *db.Handle, chapterID int64) (bool)
func ClaimDrift(h *db.Handle, claimID int64) (float64, error)
func CloseIntentOf(lemmas []string, lex *Lexicon) (string, bool)
func CountDigestSentences(text string) (int)
func CurrentClaim(h *db.Handle) (Claim, error)
func DeepRead(h *db.Handle, ch Chapter, replica string, cfg DeepReadConfig, slot Slot, logger *journal.Logger) (DeepReadResult, error)
func DefaultDeepReadConfig() (DeepReadConfig)
func DefaultWakeConfig() (WakeConfig)
func DeliverWakeup(h *db.Handle, ch Chapter, cfg WakeConfig, clock Clock, slot Slot, playerReplica string) (string, error)
func DetectClose(lem Lemmatizer, lex *Lexicon, text string) (string, bool, error)
func DetectSleep(lem Lemmatizer, lex *Lexicon, text string) (bool, error)
func EnqueueWakeForge(h *db.Handle, chapterID int64) (error)
func EnsureSession(h *db.Handle, now string) (int64, error)
func ExportBook(h *db.Handle, root string, format Format) (string, error)
func ExportChapter(h *db.Handle, root string, format Format, n int) (string, error)
func ExportReadyAll(h *db.Handle, root string) ([]string, error)
func ExportReadyChapter(h *db.Handle, root string, chapterID int64) (string, error)
func ForgeWakeup(h *db.Handle, chapterID int64, text, now string) (error)
func HasHooks(text string) (bool)
func IsSleep(lemmas []string, lex *Lexicon) (bool)
func List(h *db.Handle) ([]TocEntry, error)
func LoadDeepReadConfig(path string) (DeepReadConfig, error)
func LoadLexicon(dir string) (*Lexicon, error)
func LoadPublished(h *db.Handle, chapterID int64) (Publication, error)
func LoadWakeConfig(path string) (WakeConfig, error)
func LoadWakeup(h *db.Handle, chapterID int64) (Wakeup, error)
func MarkPeak(h *db.Handle, chapterID, sceneID int64, kind PeakKind, note, now string) (Peak, error)
func NightForgePriority(base int) (int)
func OpenChapter(h *db.Handle, day int64, now string) (Chapter, error)
func OpenChapterText(h *db.Handle, chapterID int64) (string, error)
func OpenWindow(h *db.Handle, entry WindowEntry) (Window, error)
func Process(ctx context.Context, h *db.Handle, embedder models.Embedder, chapterID int64) (error)
func PrologueText(h *db.Handle) (string, error)
func ProposeClaim(ctx context.Context, h *db.Handle, slot ClaimSlot, embedder models.Embedder) (Claim, error)
func Publish(h *db.Handle, chapterID, version int64) (error)
func ReadyDir(root, slug string) (string)
func SaveClaim(ctx context.Context, h *db.Handle, embedder models.Embedder, text string, status ClaimStatus) (Claim, error)
func SaveEpilogue(h *db.Handle, title, text string) (Chapter, error)
func ScenePeaks(h *db.Handle, sceneID int64) ([]Peak, error)
func SedimentPriority(h *db.Handle, sceneID int64) (int, error)
func SplitDigestSentences(text string) ([]string)
func TestAssemble(t *testing.T)
func TestAutoClose(t *testing.T)
func TestAutoCloseBeforeLimit(t *testing.T)
func TestAutoCloseWorldNotesSlot(t *testing.T)
func TestChapterDay(t *testing.T)
func TestClaim(t *testing.T)
func TestCloseIntents(t *testing.T)
func TestDeepRead(t *testing.T)
func TestDigest(t *testing.T)
func TestEdits(t *testing.T)
func TestEpilogue(t *testing.T)
func TestExport(t *testing.T)
func TestExportReady(t *testing.T)
func TestMain(m *testing.M)
func TestOpenChapterResetsDrift(t *testing.T)
func TestPublish(t *testing.T)
func TestPublishGate(t *testing.T)
func TestToc(t *testing.T)
func TestTrigger(t *testing.T)
func TestWake(t *testing.T)
func TestWindow(t *testing.T)
func ValidateClaim(text string) (error)
func ValidateDigest(text string) (error)
func WindowTabs() ([]WindowTab)
func WriteDigest(h *db.Handle, chapterID int64, digest string) (error)
interface ClaimSlot
interface Clock
interface DigestSlot
interface Lemmatizer
interface Slot
struct CensorCut
struct Chapter
struct ChapterView
struct Claim
struct DeepReadCitation
struct DeepReadConfig
struct DeepReadField
struct DeepReadResult
struct EditResult
struct IntentEntry
struct Lexicon
struct Peak
struct PlayerEdit
struct Publication
struct RawLogEntry
struct Session
struct TocEntry
struct Trigger
struct WakeConfig
struct WakeForgePayload
struct Wakeup
struct Window
struct WindowTab
type ChapterKind
type ChapterStatus
type ClaimStatus
type CloseReason
type DigestState
type Format
type PeakKind
type WakeStatus
type WindowEntry
var CloseIntents
var ErrChapterNotOpen
var ErrChapterNotProcessed
var ErrChapterNotPublished
var ErrClaimNotFound
var ErrClaimSentences
var ErrClaimTooFewSessions
var ErrDigestSentences
var ErrExportFormat
var ErrExportNoChapters
var ErrNoChapterVectors
var ErrNoClaim
var ErrNoDeepRead
var ErrNoPublished
var ErrNoVersion
var ErrNoWakeup
var ErrReadyExport
var ErrSessionNotOpen
var ErrTextNotProcessed
var ErrWakeHooks
```

## `_/home/ajaxii/PRJ/TNS65/engine/internal/cast`

```go
const RoleAlly
const RoleAntagonist
const RoleAuthority
const RoleBackground
const RoleMentor
const RoleProtagonist
const RoleRival
const RoleTrickster
const RoleVictim
const StatusCandidate
const StatusConfirmed
func (BookCharacter) DescriptionText() (string)
func (fakeLemmatizer) Lemmatize(text string) ([]lemmarpc.Token, error)
func (pickResolver) ResolveRole(_ context.Context, _, _ string, _ []cast.Role) (cast.Role, error)
func CharacterByID(lit *db.Handle, id int64) (BookCharacter, error)
func ChunkByOrd(lit *db.Handle, bookID, ord int64) (BookChunk, error)
func ChunkRoles(chunks []Chunk, c Candidate, dict *RoleDict, opts RoleOptions) ([]ChunkRole, error)
func DefaultRoleDict() (*RoleDict)
func Describe(chunks []Chunk, cands []Candidate, dict *DescribeDict, opts DescribeOptions) ([]Described, error)
func DescriptionByName(lit *db.Handle, name string) (string, bool)
func InsertCharacter(lit *db.Handle, c BookCharacter) (int64, error)
func InsertChunk(lit *db.Handle, c BookChunk) (int64, error)
func InsertMention(lit *db.Handle, m BookMention) (error)
func InsertRole(lit *db.Handle, r BookRole) (int64, error)
func LoadDescriptions(path string) (*DescribeDict, error)
func LoadNames(path string) ([]string, error)
func MergeRoles(rs []ChunkRole) ([]BookRole)
func Profile(lang string) (LangProfile, bool)
func PromoteTopConfirmed(lit *db.Handle, bookID int64, n int) ([]BookCharacter, error)
func RoleAt(segs []BookRole, ord int64) (Role)
func RoleList() ([]Role)
func Scan(chunks []Chunk, opts ScanOptions) ([]Candidate, error)
func TestCastCeilings(t *testing.T)
func TestCastStore(t *testing.T)
func TestDescribeDeterministic(t *testing.T)
func TestDescribeHabitVerb(t *testing.T)
func TestDescribeLoadDict(t *testing.T)
func TestDescribeNegation(t *testing.T)
func TestDescribeNeighborNotAttributed(t *testing.T)
func TestDescribeWindowExtraction(t *testing.T)
func TestDescriptionByName(t *testing.T)
func TestPromoteTopConfirmed(t *testing.T)
func TestPromoteTopConfirmedEmpty(t *testing.T)
func TestRoleListValid(t *testing.T)
func TestRolesBySegments(t *testing.T)
func TestRolesDisputedResolver(t *testing.T)
func TestRolesEmptyRole(t *testing.T)
func TestRolesNegation(t *testing.T)
func TestScanCandidates(t *testing.T)
func TestScanImportsNames(t *testing.T)
func TestScanWithoutLemmatizer(t *testing.T)
func ValidRole(s string) (bool)
interface Lemmatizer
interface RoleResolver
struct BookCharacter
struct BookChunk
struct BookMention
struct BookRole
struct Candidate
struct Chunk
struct ChunkRole
struct DescribeAxis
struct DescribeDict
struct DescribeOptions
struct Described
struct LangProfile
struct RoleDict
struct RoleEvidence
struct RoleOptions
struct ScanOptions
struct TraitEvidence
type Role
var ErrCharacterNotFound
var ErrChunkNotFound
```

## `_/home/ajaxii/PRJ/TNS65/engine/internal/censor`

```go
func (*Checker) Check(text string) (bool, []string)
func (*Checker) CheckErr(text string) (bool, []string, error)
func (*Detector) Refusal(text string) (bool, string)
func (errLemmatizer) Lemmatize(string) ([]lemmarpc.Token, error)
func (fakeLemmatizer) Lemmatize(text string) ([]lemmarpc.Token, error)
func Check(text string, lang string) (bool, []string)
func Close()
func IsRefusal(text, lang string) (bool, string)
func LoadRefusals(packDir string) ([]string, error)
func LoadStoplist(packDir string) ([]string, error)
func LoadStoplistProfile(packDir, rating string) ([]string, error)
func New(lemmatizer Lemmatizer, stoplist []string) (*Checker)
func NewDetector(markers []string) (*Detector)
func TestCheck(t *testing.T)
func TestCheckErr(t *testing.T)
func TestCheckerCache(t *testing.T)
func TestLemmaForms(t *testing.T)
func TestLoadStoplistProfile(t *testing.T)
func TestLoadStoplistProfileMissing16(t *testing.T)
func TestLoadStoplistProfileMissing21(t *testing.T)
func TestRealProfileCensor(t *testing.T)
func TestRealRefusals(t *testing.T)
func TestRealStoplistProfile(t *testing.T)
func TestRefusal(t *testing.T)
func TestStoplistGolden(t *testing.T)
interface Lemmatizer
struct Checker
struct Detector
```

## `_/home/ajaxii/PRJ/TNS65/engine/internal/core`

```go
const WorkshopSlots
func (*Scheduler) AcquireWorkshop() (release func(...))
func (*Scheduler) BeginTurn() (*TurnGuard)
func (*Scheduler) WorkshopBusy() (int)
func (*Scheduler) WorkshopWaiting() (int)
func (*TurnGuard) End()
func NewScheduler() (*Scheduler)
func TestNoImportCycles(t *testing.T)
func TestTurnPriority(t *testing.T)
struct Scheduler
struct TurnGuard
var DefaultScheduler
```

## `_/home/ajaxii/PRJ/TNS65/engine/internal/db`

```go
const DBBase
const DBLiterary
const DBWorld
const WriterBake
const WriterCore
const WriterForge
const WriterSediment
func (*ForeignWriterError) Error() (string)
func (*ForeignWriterError) Is(target error) (bool)
func (*Handle) Close() (error)
func (*Handle) EnsureMigrated() (error)
func (*Handle) Exec(table, query string, args ...any) (sql.Result, error)
func (*Handle) Query(query string, args ...any) (*sql.Rows, error)
func (*Handle) QueryRow(query string, args ...any) (*sql.Row)
func (*Handle) Tx(writer Writer, fn func(...)) (error)
func (*Tx) Exec(table, query string, args ...any) (sql.Result, error)
func Migrate(db *sql.DB, dirs ...string) (error)
func OpenReader(path string) (*sql.DB, error)
func OpenReaderAs(path string, dbName DB, writer Writer, logger *journal.Logger) (*Handle, error)
func OpenWriter(path string) (*sql.DB, error)
func OpenWriterAs(path string, dbName DB, writer Writer, logger *journal.Logger) (*Handle, error)
func TestBaseRootDefault(t *testing.T)
func TestBaseRootParametrized(t *testing.T)
func TestBaseSchema(t *testing.T)
func TestBaseWritersBakeOnly(t *testing.T)
func TestEnsureMigratedOnOpen(t *testing.T)
func TestEnsureMigratedPlayerProfile(t *testing.T)
func TestEnsureMigratedUnknownDB(t *testing.T)
func TestLiteraryDeepreadSchema(t *testing.T)
func TestLiteraryGefestRename(t *testing.T)
func TestLiterarySchema(t *testing.T)
func TestLiteraryVectors(t *testing.T)
func TestTxRollback(t *testing.T)
func TestWorldSchema(t *testing.T)
func TestWriterMap(t *testing.T)
func TestWriterMapCoverage(t *testing.T)
struct ForeignWriterError
struct Handle
struct Tx
type DB
type Writer
var BaseRoot
var ErrForeignWriter
var WorldsRoot
```

## `_/home/ajaxii/PRJ/TNS65/engine/internal/deepread`

```go
const AnswerBoundaryJoin
const AnswerBoundarySplit
const AnswerMergeDifferent
const AnswerMergeSame
const BandConfident
const BandReject
const BandUncertain
const CharacterVectorTable
const DictAnchorTable
const DictVersionLen
const DisputeAnswered
const DisputeBoundary
const DisputeCategory
const DisputeEmpty
const DisputeFailed
const DisputeMerge
const DisputeRole
const DisputeSlotRole
const DisputeUncertain
const EventDeepreadSlot
const FeatureAvgSentenceLen
const FeatureDialogueShare
const FeatureEllipsisDensity
const FeatureExclaimDensity
const FeatureLexicalDiversity
const FeatureQuestionDensity
const KeyActionDramatic
const KeyAppearance
const KeyArchitecture
const KeyBattle
const KeyClothing
const KeyEmotion
const KeyErotic
const KeyNature
const KeySocial
const KeySpeech
const KeyTime
const KeyViolence
const KindBookMap
const KindDeepread
const MapChroniclerRole
const MapFictionRole
const MapModeFiction
const MapModeMixed
const MapModeReal
const MapNavigatorRole
const MaxSeverity
const MinSeverity
const Rating16
const Rating18
const ReasonUnchangedKey
const SceneVectorTable
const StagingStatusCandidate
const StagingStatusPromoted
const StagingStatusRejected
const StyleDescriptive
const StyleDialogic
const StyleDry
const StyleDynamic
const StyleLyrical
func (*Run) ChunkLemmas(chunks []string) ([][]Lemma)
func (*Run) Close() (error)
func (*Run) Err() (error)
func (*Run) Lemmatize(text string) ([]Lemma, error)
func (*TimeLexicon) Markers(chunk SalienceChunk) ([]string)
func (*callCounter) Generate(ctx context.Context, role, prompt string) (string, error)
func (*mapSlot) Generate(_ context.Context, role, prompt string) (string, error)
func (DisputeOutcome) AppliedTag() (ChunkTag, bool)
func (Key) Equal(other Key) (bool)
func (Key) String() (string)
func (Key) Valid() (bool)
func ApplyBoundaries(set *BookScenes, outcomes []DisputeOutcome) (error)
func ApplyDecision(lit *db.Handle, out DisputeOutcome) (error)
func AttachWindowLemmas(cands []CharacterCandidate, chunks []SalienceChunk)
func AttachWindowVectors(lit, world *db.Handle, bookID int64, cands []CharacterCandidate) (error)
func BuildBookMap(ctx context.Context, lit *db.Handle, in MapInput) (*BookMap, error)
func BuildBookStyle(in StyleInput) (*BookStyle, error)
func BuildCharacters(lit, world *db.Handle, in CharacterInput) ([]CharacterGroup, []DisputedAlias, error)
func BuildDictAnchors(chunks []SalienceChunk, vectors map[int64][]float32, tags []ChunkTags, dicts map[string]*Dictionary) ([]DictAnchor)
func BuildDisputes(in DisputeInput) ([]DisputeQuestion, error)
func BuildScenes(lit, world *db.Handle, in SceneInput) (*BookScenes, error)
func BuildStagingCandidates(chunks []SalienceChunk, vectors map[int64][]float32, anchors []DictAnchor, dicts map[string]*Dictionary, cfg StagingConfig) ([]StagingCandidate)
func CategoryID(key string) (int)
func Centroid(windows []CharacterWindow) ([]float32)
func Cosine(a, b []float32) (float64)
func DefaultCharacterConfig() (CharacterConfig)
func DefaultDisputeConfig() (DisputeConfig)
func DefaultRatingConfig() (RatingConfig)
func DefaultSalienceConfig() (SalienceConfig)
func DefaultStagingConfig() (StagingConfig)
func DictVersion(packDir string) (string, error)
func DramaScore(text string, salience map[string]float64) (float64)
func FindRun(lit *db.Handle, key Key) (RunRecord, bool, error)
func IDF(chunks []SalienceChunk) (map[string]float64)
func LoadBookMap(lit *db.Handle, bookID int64) (*BookMap, bool, error)
func LoadCharacterCandidates(lit *db.Handle, bookID int64) ([]CharacterCandidate, error)
func LoadCharacterConfig(path string) (CharacterConfig, error)
func LoadDictionaries(packDir string) (map[string]*Dictionary, error)
func LoadDictionary(packDir, key string) (*Dictionary, error)
func LoadDisputeConfig(path string) (DisputeConfig, error)
func LoadEmbedModel(path string) (string, error)
func LoadEmotionDictionary(path string) (*EmotionDictionary, error)
func LoadMapSource(lit *db.Handle, bookID int64) (*MapSource, error)
func LoadRatingConfig(path string) (RatingConfig, error)
func LoadSalienceConfig(path string) (SalienceConfig, error)
func LoadStagingConfig(path string) (StagingConfig, error)
func LoadStyles(packDir string) (*Styles, error)
func LoadTimeLexicon(packDir string) (*TimeLexicon, error)
func MapRole(mode MapMode) (string, error)
func MergeCharacters(cands []CharacterCandidate, dict *EmotionDictionary, cfg CharacterConfig) ([]CharacterGroup, []DisputedAlias, error)
func NewKey(packDir, configPath, bookText string) (Key, error)
func OpenRun(packDir string) (*Run, error)
func RankDisputes(questions []DisputeQuestion)
func ResolveDisputes(questions []DisputeQuestion, cfg DisputeConfig, slot DisputeSlot, logger *journal.Logger) ([]DisputeOutcome, error)
func RunOnce(lit *db.Handle, key Key, run func(...)) (RunOutcome, error)
func RunPipeline(lit, world *db.Handle, in RunInput) (RunReport, error)
func SeverityWeight(severity int) (int, bool)
func StageDictionaries(lit, world *db.Handle, in StagingInput) (*StagingResult, error)
func TagChunks(chunks []SalienceChunk, dicts map[string]*Dictionary, cfg SalienceConfig) ([]ChunkTags, error)
func TestBuildBookMap(t *testing.T)
func TestBuildBookMapDegradation(t *testing.T)
func TestBuildBookMapEmpty(t *testing.T)
func TestCharacterCentroids(t *testing.T)
func TestDeepreadKey(t *testing.T)
func TestDictGates(t *testing.T)
func TestDisputeBudget(t *testing.T)
func TestEroticSeverity(t *testing.T)
func TestEroticSeverityValidation(t *testing.T)
func TestGoldenPrecision(t *testing.T)
func TestLemmas(t *testing.T)
func TestLoadMapSource(t *testing.T)
func TestMapRoleByTravelMode(t *testing.T)
func TestRunPipeline(t *testing.T)
func TestSalienceBands(t *testing.T)
func TestSalienceParses(t *testing.T)
func TestScenes(t *testing.T)
func TestSeverityWeights(t *testing.T)
func TestStaging(t *testing.T)
func TestStyle(t *testing.T)
func TextHash(text string) (string)
func ValidPOS(pos string) (bool)
func WriteBookStyle(lit *db.Handle, bookID int64, style *BookStyle, now string) (error)
func WriteCharacterCentroids(lit *db.Handle, groups []CharacterGroup, now string) (error)
func WriteCharacterProfiles(lit *db.Handle, groups []CharacterGroup) (error)
func WriteChunkTags(lit *db.Handle, tags []ChunkTags) (error)
func WriteDictAnchors(lit *db.Handle, anchors []DictAnchor, now string) (error)
func WriteRunLog(lit *db.Handle, key Key, chunks int, now string) (error)
func WriteScenes(lit *db.Handle, set *BookScenes, now string) (error)
func WriteStagingCandidates(lit *db.Handle, bookID int64, cands []StagingCandidate, now string) (int, int, error)
interface DisputeSlot
interface MapSlot
struct BookMap
struct BookScenes
struct BookStyle
struct CharacterCandidate
struct CharacterConfig
struct CharacterGroup
struct CharacterInput
struct CharacterWindow
struct ChunkDrama
struct ChunkTag
struct ChunkTags
struct DictAnchor
struct Dictionary
struct DisputeConfig
struct DisputeInput
struct DisputeOutcome
struct DisputeQuestion
struct DisputedAlias
struct DisputedBoundary
struct DisputedRole
struct EmotionDictionary
struct Key
struct Lemma
struct MapInput
struct MapPlace
struct MapScene
struct MapSource
struct Payload
struct Phrase
struct Place
struct RatingConfig
struct Run
struct RunChunk
struct RunInput
struct RunOutcome
struct RunRecord
struct RunReport
struct SalienceChunk
struct SalienceConfig
struct Scene
struct SceneInput
struct StagingCandidate
struct StagingConfig
struct StagingInput
struct StagingResult
struct StyleChunk
struct StyleInput
struct StyleLabel
struct Styles
struct Summary
struct TimeLexicon
type DisputeKind
type DisputeState
type EmotionProfile
type MapMode
var CategoryKeys
var DictionaryKeys
var ErrMapEmpty
var ErrMapNoSlot
var ErrNoResident
var ErrNoStore
var POSTags
var StyleLabels
```

## `_/home/ajaxii/PRJ/TNS65/engine/internal/embedserver`

```go
const DefaultContext
const DefaultHost
const DefaultPort
func (Spec) Args() ([]string)
func (Spec) Child() (supervisor.Config)
func (Spec) URL() (string)
func Check(ctx context.Context, baseURL string, logger *journal.Logger) (error)
func StatusOf(ctx context.Context, baseURL string, logger *journal.Logger) (Status)
func TestArgs(t *testing.T)
func TestCheckBadPooling(t *testing.T)
func TestCheckHealthDown(t *testing.T)
func TestCheckOK(t *testing.T)
func TestChild(t *testing.T)
struct Spec
struct Status
```

## `_/home/ajaxii/PRJ/TNS65/engine/internal/geo`

```go
const DefaultDayNormKm
const EarthRadiusKm
const MaxDayNormKm
const MinDayNormKm
func (*Geo) DayNormKm() (float64)
func (*Geo) Days(from, to Place, path *Path) (float64)
func CeilDays(days float64) (float64)
func CheckDayNorm(v float64) (error)
func CheckPrompt(prompt string) (error)
func Distance(a, b Place) (float64)
func NewGeo(dayNormKm float64) (*Geo, error)
func ReadDayNorm(h *db.Handle) (float64, error)
func TestDays(t *testing.T)
func TestDistance(t *testing.T)
func TestNoDaysColumn(t *testing.T)
func TestPromptGate(t *testing.T)
func TravelPrompt(from, to Place, distanceKm, days float64) (string)
struct Geo
struct Path
struct Place
```

## `_/home/ajaxii/PRJ/TNS65/engine/internal/journal`

```go
const EventAnchorForced
const EventBaseCorrupt
const EventChildDegraded
const EventChildOutput
const EventChildStopped
const EventDBForeignWriter
const EventDeepreadCall
const EventEditorCensorReject
const EventEditorDigestCall
const EventEditorDigestRefuse
const EventEditorDigestReply
const EventEmbedFail
const EventForgeLaunchFailed
const EventLemmaFail
const EventNetgateDeny
const EventPortgateDeny
const EventProviderExhausted
const EventProviderFailover
const EventProviderRotate
const EventSawPhaseBreak
const EventScoutCall
const EventScoutReport
const EventSlotDegraded
const EventSlotWiring
const EventSourceError
const EventSourceImport
const EventSourceWarning
const EventSwarmVersionMismatch
const EventTurnComposition
const EventTurnDegraded
const EventTurnFallback
const EventTurnLatency
const EventTurnPath
const EventTurnSawRecommendation
const EventTurnSelectorDecision
const EventTurnStage
const EventTurnTrust
const EventWebAccess
const EventWebStaticDrift
const EventWebStaticSource
const FieldRegistryChanged
const LevelInfo
func (*Logger) Event(source, event string, fields ...Field) (error)
func (*Logger) OnEvent(fn func(...))
func (*Logger) Write(p []byte) (int, error)
func (*pipeWriter) Close() (error)
func (*pipeWriter) Write(data []byte) (int, error)
func New(path string) (*Logger, error)
func Pipe(name string, w io.Writer) (io.Writer, error)
func TestChildOutput(t *testing.T)
func TestEventsUnique(t *testing.T)
func TestRotation(t *testing.T)
struct Field
struct Logger
var ErrNoWriter
```

## `_/home/ajaxii/PRJ/TNS65/engine/internal/langpack`

```go
func (*BrokenFilesError) Error() (string)
func Build(packDir, version string, pools []Pool) (error)
func List(packsDir string) ([]Listing, error)
func LoadManifest(path string) (*Manifest, error)
func Open(dir string) (*Pack, error)
func Rollback(packDir string) (string, error)
func TestBuildHasTranslate(t *testing.T)
func TestManifest(t *testing.T)
func TestManifestBrokenFile(t *testing.T)
func TestRollback(t *testing.T)
func TestRollbackBrokenTarget(t *testing.T)
struct BrokenFilesError
struct Lemmatizer
struct Listing
struct Manifest
struct Pack
struct Pool
```

## `_/home/ajaxii/PRJ/TNS65/engine/internal/lemmarpc`

```go
func (*Client) Close() (error)
func (*Client) Lemmatize(text string) ([]Token, error)
func (Token) AllParses() ([]Parse)
func NewClient(binary string, logger *journal.Logger) (*Client)
func ReadRequest(r *bufio.Reader) (*Request, error)
func ReadResponse(r *bufio.Reader) (*Response, error)
func TestClient(t *testing.T)
func TestGoldenLemmaru(t *testing.T)
func TestProtocol(t *testing.T)
func WriteRequest(w io.Writer, id int, text string) (error)
func WriteResponse(w io.Writer, id int, tokens []Token) (error)
struct Client
struct Parse
struct Request
struct Response
struct Token
var ErrTimeout
```

## `_/home/ajaxii/PRJ/TNS65/engine/internal/library`

```go
const ManifestName
const StatusError
const StatusForging
const StatusImported
const StatusParsing
const StatusPlayer
const StatusQueued
const StatusReady
func Delete(root, id string) (error)
func Import(root, name string, data []byte) (Book, error)
func Load(root string) ([]Book, error)
func ManifestPath(root string) (string)
func Save(root string, books []Book) (error)
func TestDelete(t *testing.T)
func TestDeleteNotFound(t *testing.T)
func TestImportFB2(t *testing.T)
func TestImportLaysFile(t *testing.T)
func TestImportUniqueID(t *testing.T)
func TestImportUnsupportedFormat(t *testing.T)
func TestLoadMissingManifest(t *testing.T)
struct Book
var ErrNotFound
var ErrUnsupportedFormat
```

## `_/home/ajaxii/PRJ/TNS65/engine/internal/models`

```go
const DefaultEmbedModel
const DefaultGeminiModel
const DefaultGeminiProModel
const DefaultProvider
const DefaultWeaverURL
const EmbedDim
const PathFallback
const ProviderGeminiFlash
const ProviderGeminiPro
const ProviderLocal
const WeaverInstruction
func (*Client) Embed(ctx context.Context, text string) ([]float32, error)
func (*OpenAIClient) LimitRemaining() (string)
func (*OpenAIClient) ProviderName() (string)
func (*OpenAIClient) SetProvider(name string)
func (*OpenAIClient) Stream(ctx context.Context, prompt string) (chan string, error)
func (*ProviderClient) LimitRemaining() (string)
func (*ProviderClient) ProviderName() (string)
func (*ProviderClient) SetProvider(name string)
func (*ProviderClient) Stream(ctx context.Context, prompt string) (chan string, error)
func (*Usage) AddCall(n int64)
func (*Usage) AddCost(n int64)
func (*Usage) AddTokens(n int64)
func (*Weaver) LimitRemaining() (string)
func (*Weaver) ProviderName() (string)
func (*Weaver) SetProvider(name string)
func (*Weaver) Stream(ctx context.Context, prompt string) (chan string, error)
func (*countWriter) Write(p []byte) (int, error)
func (*fakeChat) Stream(ctx context.Context, _ string) (chan string, error)
func (*fetchServer) ServeHTTP(w http.ResponseWriter, r *http.Request)
func (fatalErr) Error() (string)
func (fatalErr) Unwrap() (error)
func AppendCall(path string, rec CallRecord) (error)
func AppendUsage(path, role string, u Usage) (error)
func BuildPrompt(facts, instruction string) (string)
func CompleteChat(ctx context.Context, chat Chat, prompt, path string, usage *Usage, logger *journal.Logger, usagePath string) (text string, outPath string)
func DefaultSlotsConfig() (SlotsConfig)
func DownloadFile(ctx context.Context, spec DownloadSpec) (DownloadResult, error)
func HasReplacementTokens(text string) (bool)
func ListOpenAIModels(ctx context.Context, baseURL, key string) ([]string, error)
func LoadSlotsConfig(path string) (SlotsConfig, error)
func ModelFor(p Provider) (string)
func ModelSettingsKey(p Provider) (string)
func NewClient(embedURL string, logger *journal.Logger, model ...string) (*Client, error)
func NewOpenAIClient(baseURL, model, apiKey string) (*OpenAIClient, error)
func NewProviderClient(baseURL, model, apiKey string) (*ProviderClient, error)
func NewWeaver(weaverURL string) (*Weaver, error)
func ProviderByName(name string) (Provider)
func ProviderPresets() ([]Provider)
func ReadCalls(path string) ([]CallRecord)
func ResetProvider() (Provider)
func TestEmbedV1(t *testing.T)
func TestEmbedV1ModelFromConfig(t *testing.T)
func TestEmbedV1Normalized(t *testing.T)
func TestEmbedV1WrongDim(t *testing.T)
func TestFetchDropMidSegment(t *testing.T)
func TestFetchNoRange(t *testing.T)
func TestFetchRangedParallel(t *testing.T)
func TestFetchResumeFromPart(t *testing.T)
func TestFetchRetriesExhausted(t *testing.T)
func TestListOpenAIModels(t *testing.T)
func TestNewOpenAIClientRejectsBadURL(t *testing.T)
func TestOpenAIChunkText(t *testing.T)
func TestOpenAIStream(t *testing.T)
func TestOpenAIStreamKeyRejected(t *testing.T)
func TestPresetDefault(t *testing.T)
func TestProviderContract(t *testing.T)
func TestProviderDeniedHost(t *testing.T)
func TestWeaverCancellation(t *testing.T)
func TestWeaverPath(t *testing.T)
func TestWeaverRejectsTokens(t *testing.T)
interface Chat
interface Embedder
struct CallRecord
struct Client
struct DownloadProgress
struct DownloadResult
struct DownloadSpec
struct OpenAIClient
struct ProviderClient
struct SlotsConfig
struct Usage
struct Weaver
type Provider
var CallJournalPath
var ErrNetwork
var ErrProviderKeyRejected
var ErrProviderRefused
```

## `_/home/ajaxii/PRJ/TNS65/engine/internal/netgate`

```go
func Client(host string) (*http.Client)
func Load(path string) (error)
func Local(addr string) (*http.Client)
func SetLogger(l *journal.Logger)
func TestDeny(t *testing.T)
func TestLoadQuotedWildcard(t *testing.T)
func TestLocalHook(t *testing.T)
func TestNoRedirect(t *testing.T)
func TestOnlyGateDials(t *testing.T)
func TestWildcardSuffix(t *testing.T)
var ErrDeniedHost
```

## `_/home/ajaxii/PRJ/TNS65/engine/internal/paganel`

```go
const BuildBudgetDefault
const CorridorDone
const CorridorQueued
const CorridorRunning
const ForgeRole
const FrictionRiver
const FrictionSea
const FrictionTrakt
const PathTypeRiver
const PathTypeSea
const PathTypeTrakt
const RingDone
const RingQueued
const RingRadiusDays
const RingRunning
const RingThresholdDays
const RoleScout
const ScoutBudgetDefault
const ScoutResultSummaryLen
const ScoutTimeoutDefault
const TravelFiction
const TravelMixed
const TravelReal
func (*Budget) BuildSpend() (bool)
func (*Budget) BuildSpent() (int)
func (*Budget) ScoutSpend() (bool)
func (*Budget) ScoutSpent() (int)
func (*DaysCalc) DayNormKm() (float64)
func (*DaysCalc) Days(from, to Place) (float64)
func (*DeepScout) Provider() (string)
func (*DeepScout) Run(ctx context.Context, topics []string) (string, error)
func (*DeepScout) Spent() (int)
func (*LocalMode) ForgeCalls() (int)
func (*LocalMode) ForgeGate() (bool)
func (*LocalMode) ForgeIsLocal() (bool)
func (*LocalMode) IsLocal() (bool)
func (*LocalMode) Preset() (workshop.Provider)
func (*LocalMode) ScoutCalls() (int)
func (*LocalMode) ScoutConfig(cfg ScoutConfig) (ScoutConfig)
func (*LocalMode) ScoutEnabled(switchEnabled bool) (bool)
func (*LocalMode) ScoutGate(switchEnabled bool) (bool)
func (*Prefilter) Keep(scene sediment.Scene) (bool)
func (*RoutedScout) Fetch(ctx context.Context, h *db.Handle, topic string) (string, error)
func (*RoutedScout) Routed(mode TravelMode) (bool)
func (*RoutedScout) Run(ctx context.Context, mode TravelMode, topics []string) (string, error)
func (*Scout) Fetch(ctx context.Context, topic string) (string, error)
func (*Scout) SetBudget(b *Budget)
func (*Scout) SetClock(c scoutClock)
func (*Scout) SetURL(url string)
func (*Scout) Spent() (int)
func (*fireClock) After(time.Duration) (chan time.Time)
func (*liveSlot) LimitRemaining() (string)
func (*liveSlot) ProviderName() (string)
func (*liveSlot) Stream(ctx context.Context, prompt string) (chan string, error)
func (Path) FrictionValue() (float64)
func (TravelMode) Valid() (bool)
func (censorAdapter) Check(text string) (bool, []string)
func (fakeCensor) Check(text string) (bool, []string)
func (prefilterCensor) Check(string) (bool, []string)
func (realScoutClock) After(d time.Duration) (chan time.Time)
func AtlasViewPrompt(v AtlasView) (string)
func BuildAtlasView(h *db.Handle) (AtlasView, error)
func BuildRing(forge *db.Handle, b *Budget) (ForgeOutput, error)
func CollectRingJournal(h *db.Handle) (RingJournal, error)
func CurrentRing(h *db.Handle) (Ring, error)
func DefaultPaganelConfig() (PaganelConfig)
func DefaultScoutConfig() (ScoutConfig)
func EnqueueRing(h *db.Handle, cfg PaganelConfig) (Ring, error)
func EnsureRingForPlace(h *db.Handle, cfg PaganelConfig, x, y float64) (bool, error)
func EnsureRingOnEdge(h *db.Handle, cfg PaganelConfig) (bool, error)
func FictionDistance(phrase string, dayNormKm float64) (float64, error)
func FinishRing(h *db.Handle, id int64, progress float64) (Ring, error)
func ForgePrompt(hero HeroPosition, h *db.Handle) (string)
func ForgeRing(forge *db.Handle) (ForgeOutput, error)
func HeroNearEdge(h *db.Handle, days float64) (bool, error)
func LoadDeepScout(configPath, providersPath, settingsPath, usagePath string, logger *journal.Logger) (*DeepScout)
func LoadGeomodel(root, slug string) (*Geomodel, error)
func LoadPaganelConfig(path string) (PaganelConfig, error)
func LoadRoutedScout(configPath, providersPath, settingsPath, usagePath string, logger *journal.Logger) (*RoutedScout)
func MarkHeroPlaceKnown(core *db.Handle, day int, drawn bool) (int64, error)
func MarkPlaceKnown(core *db.Handle, placeID int64, day int, drawn bool) (error)
func NearEdge(h *db.Handle, thresholdKm float64) (bool, error)
func NewBudget(limits BudgetLimits) (*Budget)
func NewDaysCalc(dayNormKm float64, gm *Geomodel) (*DaysCalc, error)
func NewDeepScout(cfg DeepScoutConfig, slot ScoutSlot) (*DeepScout)
func NewLocalMode(preset workshop.Provider) (*LocalMode)
func NewPrefilter(h *db.Handle) (*Prefilter, error)
func NewScout(cfg ScoutConfig, client *http.Client) (*Scout)
func PlaceBeyondEdge(h *db.Handle, x, y, days float64) (bool, error)
func PromoteDrafts(forge *db.Handle) (PromotionResult, error)
func ReadBuildBudget(path string) (int, error)
func ReadDayNorm(h *db.Handle) (float64, error)
func ReadHeroPosition(h *db.Handle) (HeroPosition, error)
func ReadPlayerMap(h *db.Handle) ([]PlayerMapPlace, error)
func ReadRing(h *db.Handle, id int64) (Ring, error)
func ReadTravelMode(h *db.Handle) (TravelMode, error)
func RealDays(from, to Place, dayNormKm float64) (float64)
func RecordCorridor(core *db.Handle, plan CorridorPlan) (Corridor, error)
func ScoutTask(mode TravelMode) (providerroute.Task, error)
func SetCensor(c Censor)
func SetHeroPlace(h *db.Handle, placeID int64) (HeroPosition, error)
func SetSlot(s Slot)
func ShiftHeroPosition(h *db.Handle, dx, dy float64) (HeroPosition, error)
func ShiftMixedCanvas(places []MixedPlace, dx, dy float64) ([]MixedPlace)
func StartRing(h *db.Handle, id int64) (Ring, error)
func TestAtlasView(t *testing.T)
func TestCalibrate(t *testing.T)
func TestCorridor(t *testing.T)
func TestDays(t *testing.T)
func TestDeepScoutBudget(t *testing.T)
func TestDeepScoutErrorsLogged(t *testing.T)
func TestDeepScoutLiveCall(t *testing.T)
func TestForgeRing(t *testing.T)
func TestGeomodel(t *testing.T)
func TestLoadDeepScout(t *testing.T)
func TestLocalMode(t *testing.T)
func TestPlayerMap(t *testing.T)
func TestPosition(t *testing.T)
func TestPrefilterImpl(t *testing.T)
func TestRingJournal(t *testing.T)
func TestRings(t *testing.T)
func TestRoutedScoutTimeout(t *testing.T)
func TestRoutedScoutTravelModeRoute(t *testing.T)
func TestRoutedScoutWithoutKeys(t *testing.T)
func TestScout(t *testing.T)
func TestThreshold(t *testing.T)
func TypeFriction(pathType string) (float64)
func UpdateRingCounters(h *db.Handle, id int64, c RingCounters) (Ring, error)
func ValidPathType(pathType string) (error)
func WorldDBPath(root, slug string) (string)
interface Censor
interface ScoutSlot
interface Slot
struct AtlasHero
struct AtlasPath
struct AtlasPlace
struct AtlasRing
struct AtlasView
struct Budget
struct BudgetLimits
struct Corridor
struct CorridorPlan
struct DaysCalc
struct DeepScout
struct DeepScoutConfig
struct ForgeOutput
struct ForgeStats
struct ForgedPath
struct ForgedPlace
struct Geomodel
struct HeroPosition
struct LocalMode
struct MixedPlace
struct PaganelConfig
struct Path
struct Place
struct PlaceCoord
struct PlayerMapPlace
struct Prefilter
struct PromotionResult
struct Ring
struct RingCounters
struct RingJournal
struct RingJournalEntry
struct RoutedScout
struct Scout
struct ScoutConfig
struct Stop
struct Waypoint
type TravelMode
var ErrBuildBudgetExhausted
var ErrForgeDisabled
var ErrScoutBudgetExhausted
var ErrScoutDisabled
var ErrScoutNoAnswer
var ErrScoutNoSlot
var ErrScoutNotRouted
var ErrScoutTimeout
```

## `_/home/ajaxii/PRJ/TNS65/engine/internal/portgate`

```go
const ExitDeny
func (Whitelist) Allows(addr netip.AddrPort) (bool)
func Evaluate(wl Whitelist, sockets []Socket) (Result)
func LoadWhitelist(path string) (Whitelist, error)
func ParseWhitelist(entries []string) (Whitelist)
func Run(wl Whitelist, logger *journal.Logger) (int)
func TestFixtureExternal(t *testing.T)
func TestFixtureLoopback(t *testing.T)
func TestV6Loopback(t *testing.T)
struct Result
struct Socket
struct Whitelist
var ErrDenied
```

## `_/home/ajaxii/PRJ/TNS65/engine/internal/providercfg`

```go
const ProtocolGemini
const ProtocolLocal
const ProtocolOpenAI
func (Config) DefaultModel(name string) (string)
func (Config) Model(id string) (Model, bool)
func (Config) Provider(name string) (Provider, bool)
func (Config) Providers() ([]Provider)
func Load(path string) (Config)
func Parse(data string) (Config)
func TestLoadFileWithoutProvidersUsesBuiltinsAndInFilePrices(t *testing.T)
func TestLoadMissingFileUsesBuiltins(t *testing.T)
func TestParseProtocolInference(t *testing.T)
func TestParseProviders(t *testing.T)
func TestParseSkipsBadLines(t *testing.T)
func TestParseStringArray(t *testing.T)
func TestShippedProvidersFile(t *testing.T)
struct Config
struct Model
struct Provider
type Protocol
```

## `_/home/ajaxii/PRJ/TNS65/engine/internal/providerroute`

```go
const LocalProvider
const QuarantineWindow
const TaskChat
const TaskEditor
const TaskScoutFiction
const TaskScoutMixed
const TaskScoutReal
const TaskSources
const TaskSourcesMap
func (*RoutedProseSlot) LimitRemaining() (string)
func (*RoutedProseSlot) ProviderName() (string)
func (*RoutedProseSlot) Stream(ctx context.Context, prompt string) (chan string, error)
func (*Router) Slot(task Task) (*RoutedProseSlot)
func (*fakeClient) LimitRemaining() (string)
func (*fakeClient) ProviderName() (string)
func (*fakeClient) Stream(_ context.Context, _ string) (chan string, error)
func (*retryAfterFake) Error() (string)
func (*retryAfterFake) RetryAfter() (time.Duration)
func (Settings) RouteFor(task Task) (Route)
func ChatWeaver(configPath, settingsPath, providersPath string, logger *journal.Logger) (*RoutedProseSlot)
func DefaultChatWeaver(logger *journal.Logger) (turn.ProseSlot)
func LoadSettings(configPath, settingsPath string) (Settings, error)
func NewRouter(cfg Settings, cat providercfg.Config, logger *journal.Logger) (*Router)
func ParseRoute(s string) (Route)
func Tasks() ([]Task)
func TestChatWeaverFailoverTwoProviders(t *testing.T)
func TestChatWeaverNoKeys(t *testing.T)
func TestChatWeaverRotation(t *testing.T)
func TestChatWeaverWiringFail(t *testing.T)
func TestLoadSettingsMissingConfig(t *testing.T)
func TestLoadSettingsMissingSettings(t *testing.T)
func TestLoadSettingsRoutes(t *testing.T)
func TestParseRoute(t *testing.T)
func TestRouteForFallbacks(t *testing.T)
func TestRouterAfterFirstChunkNoRepeat(t *testing.T)
func TestRouterAllFail(t *testing.T)
func TestRouterClientBuildFailure(t *testing.T)
func TestRouterFailover(t *testing.T)
func TestRouterKeyRejectedUntilRestart(t *testing.T)
func TestRouterLocalLast(t *testing.T)
func TestRouterNoFailoverOn400(t *testing.T)
func TestRouterNoProvider(t *testing.T)
func TestRouterQuarantine429(t *testing.T)
func TestRouterRetryAfterQuarantine(t *testing.T)
func TestRouterRotation(t *testing.T)
func TestRouterTaskCursorIsolation(t *testing.T)
func TestRouterUnknownProviderDropped(t *testing.T)
func TestShippedConfigRoutes(t *testing.T)
interface ProviderClient
struct RoutedProseSlot
struct Router
struct Settings
type ClientFactory
type Route
type Task
var ErrExhausted
var ErrNoProvider
```

## `_/home/ajaxii/PRJ/TNS65/engine/internal/sediment`

```go
const EscalationNone
const EscalationRollback
const EscalationSoft
const EventAutocriticVerdict
const EventCensorReject
const EventClean
const EventDedup
const EventDupSkip
const EventIn
const EventPrefiltered
const EventWrite
const SceneStatusClosed
const SignalDislike
const SignalLike
const Source
const TokenName
const TokenPlace
const TokenPrice
const TokenThing
const TrustLevelSettled
const TrustLevelUnchecked
const TrustLevelVerified
const VerdictClean
const VerdictDegraded
const VerdictIssues
func (*Pipeline) Run(ctx context.Context, h *db.Handle, scene Scene, bindings []Binding) (WrittenScene, error)
func (*fakeAutocriticSlot) Generate(_ context.Context, role, prompt string) (string, error)
func (*fakeCensor) Check(string) (bool, []string)
func (*fakePrefilter) Keep(sediment.Scene) (bool)
func (passPrefilter) Keep(Scene) (bool)
func Autocritic(ctx context.Context, slot Slot, logger *journal.Logger, rec SedimentRecord) (SedimentRecord)
func BinderToken(t Token) (string)
func Clean(scene Scene, bindings []Binding) (SedimentRecord, error)
func DefaultDedupConfig() (DedupConfig)
func DislikeStreak(latest []SignalKind) (int)
func Escalation(streak int) (int)
func IsDuplicate(vec []float32, existing [][]float32, threshold float64) (bool)
func LoadDedupConfig(path string) (DedupConfig, error)
func MarkNeedsReview(text string) (string)
func NewPipeline(embedder models.Embedder, slot Slot, censor Censor, logger *journal.Logger, prefilters ...Prefilter) (*Pipeline)
func ParseToken(s string) (Token, error)
func SceneSignals(h *db.Handle, sceneID int64) ([]Signal, error)
func SignalGate(signals []Signal) (int, error)
func SlotTokens() ([]Token)
func TestAutocritic(t *testing.T)
func TestClean(t *testing.T)
func TestDedup(t *testing.T)
func TestDislikeEscalation(t *testing.T)
func TestGoldenCleanDedup(t *testing.T)
func TestGoldenPipeline(t *testing.T)
func TestIdempotent(t *testing.T)
func TestPrefilter(t *testing.T)
func TestRecordLinksOriginal(t *testing.T)
func TestSignalGate(t *testing.T)
func TestSlotSetClosed(t *testing.T)
func TestWrite(t *testing.T)
func VectorBlob(vec []float32) ([]byte, error)
func VectorFromBlob(blob []byte) ([]float32, error)
func Vectorize(ctx context.Context, embedder models.Embedder, cleanedText string) ([]float32, error)
func Write(h *db.Handle, cen Censor, rec SedimentRecord, vectorID int64, now string) (int64, error)
interface Censor
interface Prefilter
interface Slot
struct Binding
struct DedupConfig
struct Fact
struct Pipeline
struct Scene
struct SedimentRecord
struct Signal
struct WrittenScene
type SignalKind
type Token
var ErrCensorReject
var ErrDuplicate
var ErrNotFullScene
var ErrPrefiltered
var ErrSceneDisliked
var ErrTrustEscalation
var ErrUnknownSlotToken
```

## `_/home/ajaxii/PRJ/TNS65/engine/internal/selector`

```go
const MoodCounterpoint
const MoodEcho
const MoodNeutral
const PathAbort
const PathBank
const PathCommand
const PathFallback
const PathWeaver
const ReasonParticipants
const ReasonPlace
const ReasonTime
const RecommendBank
const RecommendNone
const RecommendWeaver
const RuleCommand
const RuleEmotion
const RuleHardFallback
const RuleRepeatable
const RuleSawRecommendation
const RuleStopCrane
const RuleUncertain
func (*BoundaryDetector) Check(s SceneSign) (Boundary)
func (*Saw) PhaseBreak()
func (*Saw) Step(threadID int64, link MoodLink) (Recommendation, error)
func (*Saw) ThreadValue(threadID int64) (float64)
func (*Saw) Total() (float64)
func (*SceneArc) Grow(rise float64) (error)
func (*SceneArc) Reset() (error)
func (*SceneArc) Value() (float64)
func (Decision) Encode() (string)
func (Rule) Name() (string)
func DefaultSawConfig() (SawConfig)
func K(p Profile) (float64)
func LoadSawConfig(path string) (SawConfig, error)
func NewBoundaryDetector() (*BoundaryDetector)
func NewSaw(profile Profile, cfg SawConfig, charID int64, h *db.Handle, logger *journal.Logger) (*Saw)
func NewSceneArc(sceneID int64, h *db.Handle) (*SceneArc)
func RecommendationCode(r Recommendation) (int)
func Select(in Input) (Decision)
func TestSaw(t *testing.T)
func TestSawTable(t *testing.T)
func TestSceneBoundary(t *testing.T)
func TestSelector(t *testing.T)
func TestSelectorEncode(t *testing.T)
func TestSelectorPriority(t *testing.T)
struct Boundary
struct BoundaryDetector
struct Decision
struct Input
struct Saw
struct SawConfig
struct SceneArc
struct SceneSign
type MoodLink
type Path
type Profile
type Recommendation
type Rule
```

## `_/home/ajaxii/PRJ/TNS65/engine/internal/structural`

```go
const DefaultDriftForbidAfter
const DefaultForceEvery
const ScopeBook
const ScopeChapter
func (Composition) Validate() (error)
func BumpUsage(h *db.Handle, scope, family string, now string) (error)
func DefaultConfig() (Config)
func FamiliesPath(engineRoot string) (string)
func IsAdjacentPair(primary, secondary string) (bool)
func LoadCompositionConfig(path string) (Config, error)
func LoadFamilies(path string) ([]Family, error)
func RegistrySHA256(fams []Family) (string)
func ResetScope(h *db.Handle, scope, now string) (error)
func Score(c Candidate, q Query, cfg Config) (float64)
func TestAdjacentPairs(t *testing.T)
func TestCompositionConfigLoaded(t *testing.T)
func TestCompositionValidate(t *testing.T)
func TestDriftCounters(t *testing.T)
func TestFamiliesPath(t *testing.T)
func TestLoadFamilies(t *testing.T)
func TestLoadFamiliesRejectsDuplicate(t *testing.T)
func TestLoadFamiliesRejectsNone(t *testing.T)
func TestLoadFamiliesRejectsWrongCount(t *testing.T)
func TestScoreComposition(t *testing.T)
func Usage(h *db.Handle, scope string) (map[string]int, error)
func ValidMoralStance(s string) (bool)
func ValidTechnique(s string) (bool)
struct Candidate
struct Composition
struct Config
struct Family
struct Query
var AdjacentPairs
var MoralStances
var Techniques
```

## `_/home/ajaxii/PRJ/TNS65/engine/internal/supervisor`

```go
const DefaultPollInterval
const DefaultReadyTimeout
func (*Lock) Release() (error)
func (*Supervisor) Run()
func (*Supervisor) Status() ([]ChildStatus)
func (*Supervisor) Stop()
func (*Supervisor) StopChild(name string) (error)
func (*Supervisor) SwitchModel(name string, args []string) (error)
func (*execProcess) Stop() (error)
func (*execProcess) Wait() (error)
func (*fakeClock) After(d time.Duration) (chan time.Time)
func (*fakeClock) Now() (time.Time)
func (*fakeProcess) Stop() (error)
func (*fakeProcess) Wait() (error)
func (*fakeRunner) Start(spec Config, stdout, stderr io.Writer) (Process, error)
func (execRunner) Start(spec Config, stdout, stderr io.Writer) (Process, error)
func (wallClock) After(d time.Duration) (chan time.Time)
func (wallClock) Now() (time.Time)
func Acquire(path string) (*Lock, error)
func CheckHealth(spec Config) (error)
func CheckPortFree(port int) (error)
func Load(path string) ([]Config, error)
func New(logger *journal.Logger, specs []Config, opts ...Option) (*Supervisor)
func TestCheckHealth(t *testing.T)
func TestCheckPortFree(t *testing.T)
func TestChildLogPath(t *testing.T)
func TestLoadChildren(t *testing.T)
func TestNoShortReadyBudget(t *testing.T)
func TestRestart(t *testing.T)
func TestSecondInstance(t *testing.T)
func TestStatus(t *testing.T)
func TestStopChild(t *testing.T)
func TestStopOrder(t *testing.T)
func TestSwitchModel(t *testing.T)
func TestSwitchModelRollback(t *testing.T)
func WithClock(c Clock) (Option)
func WithPortCheck(f func(...)) (Option)
func WithReady(f ReadyFunc) (Option)
func WithRunner(r ProcessRunner) (Option)
interface Clock
interface Process
interface ProcessRunner
struct ChildStatus
struct Config
struct Lock
struct Supervisor
type Option
type ReadyFunc
var ErrAlreadyRunning
var ErrNotReady
var ErrStopped
```

## `_/home/ajaxii/PRJ/TNS65/engine/internal/swarm`

```go
const EnvPackageVersion
func Launch(logger *journal.Logger, spec Spec, coreVersion string) (Result, error)
func TestMain(m *testing.M)
func TestTaskLifecycle(t *testing.T)
func TestVersionMismatch(t *testing.T)
struct Handshake
struct Result
struct Spec
var ErrVersionMismatch
```

## `_/home/ajaxii/PRJ/TNS65/engine/internal/testkit`

```go
const EmbedDim
func (*Clock) Advance(d time.Duration)
func (*Clock) After(d time.Duration) (chan time.Time)
func (*Clock) Now() (time.Time)
func (*Clock) PendingTimers() (int)
func (*FakeEmbed) Calls() (int)
func (*FakeEmbed) Embed(_ context.Context, text string) ([]float32, error)
func (*FakeSlot) Calls() (int)
func (*FakeSlot) FailWith(err error)
func (*FakeSlot) Generate(ctx context.Context, role, _ string) (string, error)
func (*FakeSlot) SetCallLimit(n int)
func (*FakeSlot) SetClock(c *Clock)
func (*FakeSlot) SetDelay(d time.Duration)
func (*FakeSlot) SetResponse(role, text string)
func (*SlotSpy) Calls() (int)
func (*SlotSpy) Generate(ctx context.Context, role, prompt string) (string, error)
func (*SlotSpy) Timeouts() (int)
func NewClock() (*Clock)
func NewFakeEmbed() (*FakeEmbed)
func NewFakeSlot() (*FakeSlot)
func NewSlotSpy(inner Slot) (*SlotSpy)
func TestClock(t *testing.T)
func TestFakeEmbed(t *testing.T)
func TestFakeSlot(t *testing.T)
func TestFakeSlotCallLimit(t *testing.T)
func TestSlotSpy(t *testing.T)
interface Slot
struct Clock
struct FakeEmbed
struct FakeSlot
struct SlotSpy
var ErrCallLimit
var ErrTimeout
```

## `_/home/ajaxii/PRJ/TNS65/engine/internal/textlimit`

```go
const CyrillicWeight
const DefaultLimit
const KindPlayerMessage
const KindWorldDescription
const OtherWeight
const UnitsPerToken
func (*TooLongError) Error() (string)
func Check(kind, text string, limit int) (error)
func DefaultLimits() (Limits)
func Estimate(text string) (int)
func Hint(kind string, tokens, limit int) (string)
func LoadConfig(path string) (Limits, error)
func RuneWeight(r rune) (int)
func TestCheckBoundary(t *testing.T)
func TestCheckCustomLimit(t *testing.T)
func TestEstimateLanguages(t *testing.T)
func TestLoadConfig(t *testing.T)
func TestTokenLimitJSConstants(t *testing.T)
func TestTokenLimitJSExecution(t *testing.T)
struct Limits
struct TooLongError
```

## `_/home/ajaxii/PRJ/TNS65/engine/internal/turn`

```go
const BranchBank
const BranchWeaver
const DecisionConfirmed
const DecisionMargin
const DecisionReask
const MinutesPerDay
const NewsEventKind
const NightThresholdMin
const PreselectBank
const PreselectWeaver
const ReactionChased
const ReactionFed
const ReactionIgnored
const SilenceThreshold
const TokenNPCGroup
const TokenNPCName
const TokenNPCNameActive
const TokenNPCNamePassive
const TokenNPCRole
const TokenNPCRoleDeity
const TokenPlace
func (*Lexicon) DetectFlirt(text string) (FlirtVerdict)
func (*Lexicon) DetectSarcasm(text string) (SarcasmVerdict)
func (*Pipeline) Run(ctx context.Context, in TurnInput) (TurnOutput, error)
func (*Secrets) Assign(characterID int64, sec Secret) (error)
func (*Secrets) Assigned(characterID int64) (Secret, bool, error)
func (*SlotBinder) Bind(template string, mentions []Mention, sceneID, chapterID int64) (string, []Binding)
func (*SlotBinder) DetectMentions(text string) ([]Mention)
func (*SlotBinder) Roster() (Roster)
func (*TimeDetector) Detect(text string) (TimeSpan, bool)
func (*Troupe) BookDescription(name string) (string, bool)
func (*Troupe) FirstUnfiredGun() (gun, bool, error)
func (*Troupe) LoadRoster() (Roster, error)
func (*Troupe) NextElement(label string) (gun, error)
func (*Troupe) RecordMeeting(characterID int64, reaction string) (error)
func (*Troupe) RegisterGun(g gun) (error)
func (*Troupe) Spawn(characterID int64) (NPC, error)
func (*Troupe) WithLiterary(lit *db.Handle) (*Troupe)
func (*Troupe) WithNamePool(baseDBPath, lang string) (*Troupe)
func (*WorldClock) ElapsedMinutes() (int64)
func (*WorldClock) Now() (int64)
func (*auditStage) Name() (string)
func (*auditStage) Run(_ context.Context, st *TurnState) (error)
func (*commitStage) Name() (string)
func (*commitStage) Run(_ context.Context, st *TurnState) (error)
func (*compositionStage) Name() (string)
func (*compositionStage) Run(_ context.Context, st *TurnState) (error)
func (*confirmStage) Name() (string)
func (*confirmStage) Run(ctx context.Context, st *TurnState) (error)
func (*cutStage) Name() (string)
func (*cutStage) Run(ctx context.Context, st *TurnState) (error)
func (*deathStage) Name() (string)
func (*deathStage) Run(_ context.Context, st *TurnState) (error)
func (*digestStage) Name() (string)
func (*digestStage) Run(ctx context.Context, st *TurnState) (error)
func (*executeStage) Name() (string)
func (*executeStage) Run(ctx context.Context, st *TurnState) (error)
func (*fakeProseSlot) Stream(_ context.Context, prompt string) (chan string, error)
func (*feedbackStage) Name() (string)
func (*feedbackStage) Run(_ context.Context, st *TurnState) (error)
func (*gatedSlot) Stream(_ context.Context, _ string) (chan string, error)
func (*metaProseSlot) LimitRemaining() (string)
func (*metaProseSlot) ProviderName() (string)
func (*notifiedClock) Advance(d time.Duration)
func (*notifiedClock) After(d time.Duration) (chan time.Time)
func (*positionStage) Name() (string)
func (*positionStage) Run(_ context.Context, st *TurnState) (error)
func (*prepathStage) Name() (string)
func (*prepathStage) Run(_ context.Context, st *TurnState) (error)
func (*returnStage) Name() (string)
func (*returnStage) Run(_ context.Context, st *TurnState) (error)
func (*runawayClock) After(d time.Duration) (chan time.Time)
func (*runawayClock) Now() (time.Time)
func (*safetyStage) Name() (string)
func (*safetyStage) Run(_ context.Context, st *TurnState) (error)
func (*secretStage) Name() (string)
func (*secretStage) Run(_ context.Context, st *TurnState) (error)
func (*selectStage) Name() (string)
func (*selectStage) Run(_ context.Context, st *TurnState) (error)
func (*timejumpStage) Name() (string)
func (*timejumpStage) Run(_ context.Context, st *TurnState) (error)
func (*troupeStage) Name() (string)
func (*troupeStage) Run(_ context.Context, st *TurnState) (error)
func (Confirmation) Confirmed() (bool)
func (FeedbackPlan) Regenerating() (bool)
func (FeedbackPlan) RollbackReady() (bool)
func (constEmbed) Embed(_ context.Context, _ string) ([]float32, error)
func (fakeConfirmEmbed) Embed(_ context.Context, _ string) ([]float32, error)
func (fakeLemmatizer) Lemmatize(text string) ([]lemmarpc.Token, error)
func (realClock) After(d time.Duration) (chan time.Time)
func (realClock) Now() (time.Time)
func (wallClock) After(d time.Duration) (chan time.Time)
func (wordLemmatizer) Lemmatize(text string) ([]lemmarpc.Token, error)
func DefaultConfirmConfig() (ConfirmConfig)
func DefaultNewsSceneConfig() (NewsSceneConfig)
func DefaultTimeConfig() (TimeConfig)
func LoadConfirmConfig(path string) (ConfirmConfig, error)
func LoadLexicon(dir string) (*Lexicon, error)
func LoadTimeConfig(path string) (TimeConfig, error)
func LoadTimeDetector(dir string) (*TimeDetector, error)
func NewSecrets(h *db.Handle) (*Secrets)
func NewSlotBinder(r Roster) (*SlotBinder)
func NewTroupe(h *db.Handle) (*Troupe)
func NewWorldClock(startMin int64, src ClockSource) (*WorldClock)
func PickPoolName(basePath, lang string, taken map[string]bool) (string, error)
func ProviderSlot(def providercfg.Provider, apiKey, model string) (ProseSlot, error)
func SecretFor(name string) (Secret, bool)
func TestAbsence(t *testing.T)
func TestAudit(t *testing.T)
func TestCommitTurnWritesPosition(t *testing.T)
func TestCompositionBookBoost(t *testing.T)
func TestCompositionStage(t *testing.T)
func TestConfirm(t *testing.T)
func TestConfirmConfigLoaded(t *testing.T)
func TestCut(t *testing.T)
func TestCutNoLLM(t *testing.T)
func TestDeathThreshold(t *testing.T)
func TestDetectMoodPack(t *testing.T)
func TestDetectMoodVectors(t *testing.T)
func TestDigest(t *testing.T)
func TestFeedbackEscalation(t *testing.T)
func TestFlirtDefaults(t *testing.T)
func TestFlirtDictionaryGate(t *testing.T)
func TestFlirtRealPack(t *testing.T)
func TestNamePool(t *testing.T)
func TestNewsSceneChoiceDeterministic(t *testing.T)
func TestNewsSceneDegradation(t *testing.T)
func TestNewsSceneDelivery(t *testing.T)
func TestNewsScenePrompt(t *testing.T)
func TestNewsSceneRegistered(t *testing.T)
func TestPipelineDelivers(t *testing.T)
func TestPosition(t *testing.T)
func TestPrepath(t *testing.T)
func TestPrepathDetectMoodSignals(t *testing.T)
func TestProviderSlotProtocols(t *testing.T)
func TestSarcasmBroken(t *testing.T)
func TestSarcasmCoefficients(t *testing.T)
func TestSarcasmMarkers(t *testing.T)
func TestSarcasmOptional(t *testing.T)
func TestSarcasmRealPack(t *testing.T)
func TestSecrets(t *testing.T)
func TestSelectorTurns(t *testing.T)
func TestSilenceThreshold(t *testing.T)
func TestSlashCommands(t *testing.T)
func TestSlotBinder(t *testing.T)
func TestStopCrane(t *testing.T)
func TestStream(t *testing.T)
func TestTimeJump(t *testing.T)
func TestTroupe(t *testing.T)
func TestTroupeBookDescription(t *testing.T)
func TestTroupeStage(t *testing.T)
func TestWeaverCallJournal(t *testing.T)
interface Clock
interface ClockSource
interface Lemmatizer
interface NewsSceneSlot
interface ProseSlot
interface Stage
struct AbsenceReport
struct AssignedSecret
struct Binding
struct Command
struct ConfirmConfig
struct Confirmation
struct Cut
struct Digest
struct FateVector
struct FeedbackPlan
struct FlirtAntiTriggers
struct FlirtCategory
struct FlirtDictionary
struct FlirtMeta
struct FlirtModifiers
struct FlirtVerdict
struct HeroPosition
struct Intent
struct IntentEntry
struct Lexicon
struct Mention
struct MoodAxis
struct MoodEntry
struct MoodMeta
struct MoodModifiers
struct MoodScale
struct MoodSignals
struct NPC
struct NewsScene
struct NewsSceneConfig
struct Pipeline
struct Place
struct PlaceCoord
struct Portion
struct PositionShift
struct Prepath
struct Roster
struct SarcasmCoefficients
struct SarcasmContrastCategory
struct SarcasmDictionary
struct SarcasmMeta
struct SarcasmPatterns
struct SarcasmPhraseCategory
struct SarcasmVerdict
struct SceneFrame
struct Secret
struct Secrets
struct SedimentCandidate
struct Segment
struct SlotBinder
struct TimeConfig
struct TimeDetector
struct TimeJump
struct TimeSpan
struct Troupe
struct TurnInput
struct TurnOutput
struct TurnState
struct WordVector
struct WorldClock
type Preselect
```

## `_/home/ajaxii/PRJ/TNS65/engine/internal/version`

```go
var BuildDate
var Version
```

## `_/home/ajaxii/PRJ/TNS65/engine/internal/webapi`

```go
const DefaultHost
const DefaultPort
const DefaultTunnelEdge
const DefaultTunnelLocal
func (*Hub) Broadcast(msg WSMessage)
func (*Hub) Close()
func (*Server) Addr() (string)
func (*Server) Close() (error)
func (*Server) Hub() (*Hub)
func (*Server) Listen() (error)
func (*Server) Serve() (error)
func (*TelegramBot) ForwardChat(ctx context.Context, text string) (int64, error)
func (*TelegramBot) GetUpdates(ctx context.Context, offset int64) ([]telegramUpdate, error)
func (*TelegramBot) Poll(ctx context.Context) (error)
func (*TelegramBot) PollOnce(ctx context.Context, offset int64) (int64, error)
func (*TelegramBot) SendChat(ctx context.Context, text string) (error)
func (*TelegramBot) SendNews(ctx context.Context, text string) (error)
func (*TelegramBot) SubscribeNews(ctx context.Context, onNews func(...)) (error)
func (*router) ServeHTTP(w http.ResponseWriter, r *http.Request)
func (*statusWriter) Hijack() (net.Conn, *bufio.ReadWriter, error)
func (*statusWriter) Write(b []byte) (int, error)
func (*statusWriter) WriteHeader(code int)
func (TunnelConfig) Command() ([]string)
func (TunnelConfig) Validate() (error)
func (passCensor) Check(string) (bool, []string)
func (wallChatClock) After(d time.Duration) (chan time.Time)
func (wallChatClock) Now() (time.Time)
func DefaultPackLemmatizer(logger *journal.Logger) (turn.Lemmatizer)
func DefaultTunnelConfig() (TunnelConfig)
func Listen(host string, port int) (net.Listener, error)
func LoadBookWindow(h *db.Handle, entry book.WindowEntry) (BookWindow, error)
func LoadBotConfig(tunnelPath, settingsPath string) (TunnelConfig, error)
func LoadConfig(path string) (Config, error)
func LoadTunnelConfig(path string) (TunnelConfig, error)
func LogTelegramConnect(logger *journal.Logger, cfg TunnelConfig)
func LogTunnelConnect(logger *journal.Logger, cfg TunnelConfig)
func ModelChildName(role string) (string)
func ModelChildren() ([]supervisor.Config)
func New(cfg Config) (*Server)
func NewHandler(cfg Config) (http.Handler)
func NewHub() (*Hub)
func NewTelegramBot(cfg TunnelConfig, logger *journal.Logger) (*TelegramBot)
func SetChatLemmatizer(l turn.Lemmatizer)
func SetChatWeaver(s turn.ProseSlot)
func SetForgeProgress(slug, name string, progress float64) (error)
func SetModelChildStop(f func(...))
func SetModelChildSwitch(f func(...))
func SetModelLogger(l *journal.Logger)
func StartTelegram(ctx context.Context, cfg TunnelConfig, logger *journal.Logger) (error)
func TestActiveWorldAutoSelectSingle(t *testing.T)
func TestActiveWorldNoAutoSelectTwoWorlds(t *testing.T)
func TestActiveWorldRestart(t *testing.T)
func TestActiveWorldRestoreBroken(t *testing.T)
func TestAtlasBirth(t *testing.T)
func TestAtlasBirthFanficSourceBook(t *testing.T)
func TestAtlasBirthFieldErrors(t *testing.T)
func TestAtlasBirthOptions(t *testing.T)
func TestAtlasDelete(t *testing.T)
func TestAtlasForgeFanfic(t *testing.T)
func TestAtlasForgeNoFanfic(t *testing.T)
func TestAtlasForgeNoModel(t *testing.T)
func TestAtlasForgePromotesFanfic(t *testing.T)
func TestAtlasList(t *testing.T)
func TestAtlasReplay(t *testing.T)
func TestAtlasSwitch(t *testing.T)
func TestBookDigest(t *testing.T)
func TestBookExport(t *testing.T)
func TestBookImport(t *testing.T)
func TestBookImportBroken(t *testing.T)
func TestBookImportCharset(t *testing.T)
func TestBookImportTxtCharsets(t *testing.T)
func TestBookWindow(t *testing.T)
func TestCensorCuts(t *testing.T)
func TestChannelGetHidesToken(t *testing.T)
func TestChannelPutClearTokenStopsBot(t *testing.T)
func TestChannelPutRestartErrorWarning(t *testing.T)
func TestChannelPutRestartsBotOnce(t *testing.T)
func TestChannelPutRoundTrip(t *testing.T)
func TestChannelQuotedEmptyToken(t *testing.T)
func TestChannelRejectsBadInput(t *testing.T)
func TestChannelStatusesUseGlyphs(t *testing.T)
func TestChatState(t *testing.T)
func TestChatStateHeroHealthAndPlace(t *testing.T)
func TestChatStateReturnNews(t *testing.T)
func TestChatStateReturnNewsAfterAbsence(t *testing.T)
func TestChildEmbedRollback(t *testing.T)
func TestChildModelSpecs(t *testing.T)
func TestChildStartAfterInstall(t *testing.T)
func TestCoeffsJournal(t *testing.T)
func TestCoeffsMath(t *testing.T)
func TestCommandHelp(t *testing.T)
func TestCommandModelBadProvider(t *testing.T)
func TestCommandModelBadRole(t *testing.T)
func TestCommandModelList(t *testing.T)
func TestCommandModelSwitch(t *testing.T)
func TestCommandStats(t *testing.T)
func TestCommandUnknown(t *testing.T)
func TestCommandWorldCurrent(t *testing.T)
func TestCommandWorldSwitch(t *testing.T)
func TestCommandWorlds(t *testing.T)
func TestDayNormKmTunable(t *testing.T)
func TestDegradationCounter(t *testing.T)
func TestEmbedDim1024(t *testing.T)
func TestEmbedModelInstallsIntoEmbedDir(t *testing.T)
func TestFirstRunCompletesAtStep4(t *testing.T)
func TestFirstRunNoFileHashing(t *testing.T)
func TestFirstRunRejectsBadStep(t *testing.T)
func TestFirstRunStartsIncomplete(t *testing.T)
func TestFirstRunStepIdempotent(t *testing.T)
func TestForgeDash(t *testing.T)
func TestHealth(t *testing.T)
func TestHealthWord(t *testing.T)
func TestLLMSettingsConfig(t *testing.T)
func TestLLMSettingsUnquotesAddr(t *testing.T)
func TestLemmatizerCard(t *testing.T)
func TestLibraryDelete(t *testing.T)
func TestLibraryDeleteNotFound(t *testing.T)
func TestLibraryImportAndList(t *testing.T)
func TestLibraryImportUnsupported(t *testing.T)
func TestLibraryListCast(t *testing.T)
func TestLlamaInstallConflictWithoutNetgate(t *testing.T)
func TestLlamaInstallDetectEmpty(t *testing.T)
func TestLlamaInstallDetectsCandidateBinary(t *testing.T)
func TestLlamaInstallDetectsPathBinary(t *testing.T)
func TestLlamaInstallInstallWithoutNetwork(t *testing.T)
func TestLlamaInstallKeepsConfigOnDetect(t *testing.T)
func TestLlamaInstallManualPath(t *testing.T)
func TestLlamaInstallRunsScriptWhenEmpty(t *testing.T)
func TestLlamaInstallScanFindsBinary(t *testing.T)
func TestLoadBotConfigMergesTelegramToken(t *testing.T)
func TestLoopbackOnly(t *testing.T)
func TestMain(m *testing.M)
func TestMapViewport(t *testing.T)
func TestMcpForgeMigratesWorld(t *testing.T)
func TestModelAdoptBadRequests(t *testing.T)
func TestModelAdoptBusyRole(t *testing.T)
func TestModelAdoptFile(t *testing.T)
func TestModelAdoptFromDownloads(t *testing.T)
func TestModelAdoptMissingFile(t *testing.T)
func TestModelAdoptZeroByteInDownloads(t *testing.T)
func TestModelChildName(t *testing.T)
func TestModelDelete(t *testing.T)
func TestModelInstallFileNaming(t *testing.T)
func TestModelInstallRollback(t *testing.T)
func TestModelInstallWithoutWorld(t *testing.T)
func TestModelRollbackIntegrity(t *testing.T)
func TestModelStatusDirs(t *testing.T)
func TestModelStatusEndpoint(t *testing.T)
func TestModelStatusJobVariant(t *testing.T)
func TestMonitorReadOnly(t *testing.T)
func TestMonitorRolesAndLimit(t *testing.T)
func TestNoAuth(t *testing.T)
func TestNoWorldWriteRoutes(t *testing.T)
func TestParseCommand(t *testing.T)
func TestParseMemTotalKB(t *testing.T)
func TestParseTunnelUnquotes(t *testing.T)
func TestProviderCardsDynamicProvider(t *testing.T)
func TestProviderCardsReadProvidersFileLive(t *testing.T)
func TestProviderCatalogMissingFile(t *testing.T)
func TestProviderCatalogPriceForDisabledModel(t *testing.T)
func TestProviderCost(t *testing.T)
func TestProviderModelCardsCatalogDisplayName(t *testing.T)
func TestProviderModelIDSlashes(t *testing.T)
func TestProviderModelSave(t *testing.T)
func TestProviderModelsCuratedList(t *testing.T)
func TestProviderModelsList(t *testing.T)
func TestProviderModelsOpenAI(t *testing.T)
func TestProviderQuota(t *testing.T)
func TestProviderRoutesAPI(t *testing.T)
func TestProviderRoutesWarnings(t *testing.T)
func TestRelationGraph(t *testing.T)
func TestRingsCard(t *testing.T)
func TestRolePromptsPersisted(t *testing.T)
func TestRouteTable(t *testing.T)
func TestRouteTableLimits(t *testing.T)
func TestRouteTablePaths(t *testing.T)
func TestSceneSignal(t *testing.T)
func TestServeStatic(t *testing.T)
func TestSettingsBindingForget(t *testing.T)
func TestSettingsBindingsValidated(t *testing.T)
func TestSettingsGetWithoutWorld(t *testing.T)
func TestSettingsReportsRamGB(t *testing.T)
func TestSettingsSections(t *testing.T)
func TestSettingsSupervisorStatus(t *testing.T)
func TestSlotBindings(t *testing.T)
func TestSourceReforge(t *testing.T)
func TestSplitReply(t *testing.T)
func TestStartTelegramLifecycle(t *testing.T)
func TestStartTelegramNotConfigured(t *testing.T)
func TestStaticDefaultDirIsBinaryDir(t *testing.T)
func TestStaticDiskFirst(t *testing.T)
func TestStaticEmbedFallback(t *testing.T)
func TestStaticIncompleteDirFallsBack(t *testing.T)
func TestStaticManifestDrift(t *testing.T)
func TestTelegramBotDislikeCallback(t *testing.T)
func TestTelegramBotForeignCallbackIgnored(t *testing.T)
func TestTelegramBotForeignChatIgnored(t *testing.T)
func TestTelegramBotForwardsChat(t *testing.T)
func TestTelegramBotLikeCallback(t *testing.T)
func TestTelegramBotLogsConnect(t *testing.T)
func TestTelegramBotLongPoll(t *testing.T)
func TestTelegramBotLoopbackErrorDoesNotStopPoll(t *testing.T)
func TestTelegramBotNoActiveWorld(t *testing.T)
func TestTelegramBotRelaysTooLongHint(t *testing.T)
func TestTelegramBotRoundTrip(t *testing.T)
func TestTelegramBotSignalErrorAnswer(t *testing.T)
func TestTelegramBotStaleTurnReplyNotSent(t *testing.T)
func TestTelegramBotSubscribeNews(t *testing.T)
func TestTelegramBotUnknownCallbackDataIgnored(t *testing.T)
func TestTunablesRoute(t *testing.T)
func TestTunablesWrite(t *testing.T)
func TestTunnelOutboundOnly(t *testing.T)
func TestTurnEndpoint(t *testing.T)
func TestTurnEndpointTooLong(t *testing.T)
func TestTurnStream(t *testing.T)
func TestUITextsWithoutStationSlang(t *testing.T)
func TestWireWorldRating(t *testing.T)
func TestWizardCheckDotHasNoCircle(t *testing.T)
func TestWizardScreenServed(t *testing.T)
func TestWizardStationChecksUseGlyphs(t *testing.T)
func TestWizardTextsForOwner(t *testing.T)
func TestWriteSettingsKeysMode(t *testing.T)
struct BookCensorCut
struct BookChapter
struct BookRawLogEntry
struct BookTab
struct BookTocEntry
struct BookWindow
struct Config
struct Hub
struct Server
struct TelegramBot
struct TunnelConfig
struct WSMessage
var ErrExternalBind
var ErrHostNotAllowed
var ErrNoActiveWorld
var ErrTelegramNotConfigured
var ErrWSHandshake
var StopTelegram
var SupervisorStatus
var TelegramRestart
```

## `_/home/ajaxii/PRJ/TNS65/engine/internal/workshop`

```go
const BookStatusPlayer
const CastRole
const ComposeRole
const DefaultArcsPath
const DefaultLuck
const DefaultProvider
const DeferredFired
const DeferredKindCorridorStop
const DeferredPending
const EditorDigestRole
const EditorRole
const EpochRole
const ForgeBookKind
const ForgeBookRole
const ForgeBookStatusReady
const ForgeCardKind
const ForgeCardRole
const FormatFB2
const FormatTXT
const HoroscopeKind
const HoroscopeRole
const KindAnalysis
const KindComplement
const KindEdit
const KindEpoch
const KindForgePlan
const KindHoroscope
const KindPythia
const KindRing
const KindSediment
const KindStyle
const KindWakeup
const MaxChunkRunes
const PauseHeavy
const PauseLight
const PauseMedium
const PauseNight
const PauseNone
const ProviderGeminiFlash
const ProviderGeminiPro
const ProviderLocal
const PythiaRole
const Rating16Plus
const Rating18Plus
const Rating21Plus
const SedimentCensorReject
const SedimentDupSkip
const SedimentPrefiltered
const SedimentWrite
const StatusCancelled
const StatusDone
const StatusFailed
const StatusQueued
const StatusRunning
const StepHeavy
const StepLight
const StyleBlockHeader
const StyleRole
func (*DeferredTimer) CorridorStop(dueDay int64) (error)
func (*DeferredTimer) FireDue() (int, error)
func (*LocalSource) Fetch(_ context.Context, topic string) (string, error)
func (*Luck) Draw() (float64)
func (*Luck) Get() (float64, error)
func (*Luck) Roll(p float64) (Roll, error)
func (*Luck) Set(v float64) (error)
func (*Order) RequeueAllowed() (bool)
func (*PauseTimer) MarkActive()
func (*PauseTimer) Silence() (time.Duration)
func (*PauseTimer) Step(chapterClosed bool) (PauseStep)
func (*RoutedWorkshopSlot) Generate(ctx context.Context, role, prompt string) (string, error)
func (*RoutedWorkshopSlot) RouteRoles(routes RouteSlot, roles ...string) (*RoutedWorkshopSlot)
func (*Scheduler) MarkActive()
func (*Scheduler) Next() (*Order, error)
func (*Scheduler) Run(ctx context.Context) (error)
func (*Scheduler) SetChapterClosed(closed bool)
func (*Scheduler) SetSedimentRunner(run SedimentRunFunc)
func (*Scheduler) Step() (PauseStep)
func (*bookMapRoleSlot) Generate(ctx context.Context, role, prompt string) (string, error)
func (*countingCensor) Calls() (int)
func (*countingCensor) Check(text string) (bool, []string)
func (*countingSource) Fetch(_ context.Context, _ string) (string, error)
func (*fakeRouteSlot) Stream(_ context.Context, prompt string) (chan string, error)
func (*ladderSlot) Generate(_ context.Context, _ string, prompt string) (string, error)
func (*recordSlot) Generate(_ context.Context, role, _ string) (string, error)
func (*roleCountingSlot) Generate(ctx context.Context, role, prompt string) (string, error)
func (*scriptedSlot) Generate(_ context.Context, _, _ string) (string, error)
func (ResearchSourceFunc) Fetch(ctx context.Context, topic string) (string, error)
func (censorAdapter) Check(text string) (bool, []string)
func (checkerCensor) Check(text string) (bool, []string)
func (epochCensor) Check(text string) (bool, []string)
func (fakeCensor) Check(text string) (bool, []string)
func (fakeEditLemmatizer) Lemmatize(text string) ([]lemmarpc.Token, error)
func (passCensor) Check(string) (bool, []string)
func (realDeferredClock) Now() (time.Time)
func (realPauseClock) Now() (time.Time)
func ActiveRating() (string)
func BookByID(lit *db.Handle, id int64) (Book, error)
func CanRun(k Kind, orderStep Step, pause PauseStep) (bool)
func ClassifyFamily(text string, fams []structural.Family) (string)
func CollectSamples(world *db.Handle, cfg StyleConfig) ([]StyleSample, error)
func Compose(ctx context.Context, lit *db.Handle, text string, fams []structural.Family, codeFamily string) (structural.Composition, error)
func DefaultHoroscopeMatchConfig() (HoroscopeMatchConfig)
func DefaultStyleConfig() (StyleConfig)
func Delexify(sourceSceneID int64, text string, facts []sediment.Fact, bindings []sediment.Binding) (string, []string, error)
func DequeueEditorTask(world *db.Handle) (*EditorQueueTask, error)
func DeriveEpoch(description string) (string)
func EditChapter(ctx context.Context, world *db.Handle, slot Slot, chapterID int64) (EditorResult, error)
func EditorRoles() ([]string)
func Enqueue(h *db.Handle, kind Kind, step Step, provider Provider, payload any) (error)
func EnqueueBirthOrders(h *db.Handle) (error)
func EnqueueEditorTask(world *db.Handle, chapterID int64) (int64, error)
func EnsureBook(lit *db.Handle, name string) (int64, error)
func EnsureComplementOrder(h *db.Handle, arc Arc, sceneID int64, pause PauseStep) (bool, error)
func EnsureHoroscopeOrder(h *db.Handle, payload HoroscopePayload) (bool, error)
func EnsurePlayerBook(lit *db.Handle, name, lang string) (int64, error)
func EpochFromWorld(world *db.Handle) (string, error)
func EpochPrompt(epoch, material string) (string)
func FactCheck(world *db.Handle, lit *db.Handle, facts []sediment.Fact) ([]string, error)
func ForgeBankRecord(world *db.Handle, lit *db.Handle, m ForgeMaterial) (int64, error)
func ForgeBookSpec(exe string, p ForgeBookPayload) (swarm.Spec)
func ForgeCardPrompt(p ComplementPayload) (string)
func ForgeComposition(ctx context.Context, world, lit *db.Handle, m ForgeMaterial, fams []structural.Family) (int64, error)
func ForgeEpoch(ctx context.Context, world *db.Handle, lit *db.Handle, source ResearchSource) (int64, error)
func GenSlot(p Provider) (string)
func GenerateWithRetry(ctx context.Context, lit *db.Handle, opts GenerateOptions, prompt PromptFunc) (string, error)
func GenerationSlot() (Slot)
func HasMoralBlock(reply string) (bool)
func HoroscopePrompt(p HoroscopePayload) (string)
func InsertBook(lit *db.Handle, b Book) (int64, error)
func KindHasProse(k Kind) (bool)
func KindPriority(k Kind) (int)
func LoadArcs(path string) ([]Arc, error)
func LoadHoroscopeMatchConfig(path string) (HoroscopeMatchConfig, error)
func LoadStyleConfig(path string) (StyleConfig, error)
func MatchHoroscope(world, literary *db.Handle, sceneID int64, early [][]float32, cfg HoroscopeMatchConfig) (*HoroscopeMatch, error)
func NewDeferredTimer(h *db.Handle, clock DeferredClock) (*DeferredTimer)
func NewLocalSource(world *db.Handle, books []string) (*LocalSource, error)
func NewLuck(h *db.Handle, slug string) (*Luck)
func NewPauseTimer(c PauseClock) (*PauseTimer)
func NewRoutedWorkshopSlot(routes RouteSlot) (*RoutedWorkshopSlot)
func NewScheduler(h *db.Handle, clock PauseClock, slot Slot, logger *journal.Logger) (*Scheduler)
func NewSedimentRunner(lit *db.Handle, pipeline *sediment.Pipeline) (SedimentRunFunc)
func NormalizeBookFile(path string) (Transcode, error)
func ParseBookInput(path string) (*ParsedBook, error)
func PauseStepFor(silence time.Duration, chapterClosed bool) (PauseStep)
func Presets() ([]Preset)
func ProviderForRating(rating string, requested Provider) (Provider)
func ProviderForWorld(world *db.Handle, requested Provider) (Provider, error)
func ReadEpochReference(lit *db.Handle) ([]EpochReference, error)
func ReconcileComplement(h *db.Handle, arcs []Arc) (error)
func RemeltScene(lit *db.Handle, sceneID int64) (int64, error)
func ResolveCastRole(ctx context.Context, lit *db.Handle, name, chunkText string, tied []string) (string, error)
func RunEditorTask(ctx context.Context, world *db.Handle, slot Slot, task *EditorQueueTask) (EditorResult, error)
func RunForgeBook(ctx context.Context, world, lit *db.Handle, inputPath string, opts ForgeBookOptions) (ForgeBookResult, error)
func RunForgeCard(literary *db.Handle, payload ComplementPayload) (string, error)
func RunHoroscopeForge(world *db.Handle, literary *db.Handle, payload HoroscopePayload) (int64, error)
func RunPythia(world *db.Handle, literary *db.Handle, day int) (error)
func RunSedimentOrder(ctx context.Context, world *db.Handle, lit *db.Handle, pipeline *sediment.Pipeline, o *Order, logger *journal.Logger) (SedimentOutcome, error)
func SetCensor(c Censor)
func SetEmbedder(e models.Embedder)
func SetLemmatizer(l Lemmatizer)
func SetLogger(l *journal.Logger)
func SetRating(rating string)
func SetScheduler(s *core.Scheduler)
func SetSlot(s Slot)
func StepAllowed(orderStep Step, pause PauseStep) (bool)
func StyleBlock(samples []StyleSample) (string)
func StylePrompt(base string, samples []StyleSample) (string)
func StylizeClassic(ctx context.Context, lit *db.Handle, draft string) (int64, error)
func TestBirthOrders(t *testing.T)
func TestBookCharsetNormalize(t *testing.T)
func TestBookMapFallsBackToSourcesRoute(t *testing.T)
func TestBookMapThroughSourcesMapRoute(t *testing.T)
func TestBookMapWithoutKeys(t *testing.T)
func TestBookRegistry(t *testing.T)
func TestBuildDigestThroughEditorRoute(t *testing.T)
func TestComplement(t *testing.T)
func TestDeferred(t *testing.T)
func TestEditCensor(t *testing.T)
func TestEditChapterThroughEditorRoute(t *testing.T)
func TestEditor(t *testing.T)
func TestEditorJournal(t *testing.T)
func TestEditorJournalRefuse(t *testing.T)
func TestEditorQueue(t *testing.T)
func TestEditorRetry(t *testing.T)
func TestEpoch(t *testing.T)
func TestForgeBookCastFixture(t *testing.T)
func TestForgeBookChunkLimit(t *testing.T)
func TestForgeBookDeepreadSeparateStores(t *testing.T)
func TestForgeBookIgnoresCensor(t *testing.T)
func TestForgeBookThroughSourcesRoute(t *testing.T)
func TestForgeBookWithoutKeys(t *testing.T)
func TestForgeComposition(t *testing.T)
func TestForgeInput(t *testing.T)
func TestForgePipeline(t *testing.T)
func TestGenerateWithRetryLadder(t *testing.T)
func TestGenerateWithRetryTwoPass(t *testing.T)
func TestGlobalLuck(t *testing.T)
func TestHoroscopeForge(t *testing.T)
func TestHoroscopeMatch(t *testing.T)
func TestParseFB2CharsetReader(t *testing.T)
func TestParseFB2DeclarationLie(t *testing.T)
func TestPauseBoundaries(t *testing.T)
func TestPauseSteps(t *testing.T)
func TestProviderForRating(t *testing.T)
func TestProviderForWorldRating18RestrictsToLocal(t *testing.T)
func TestPythia(t *testing.T)
func TestQueueKinds(t *testing.T)
func TestRatingProfiles(t *testing.T)
func TestResolveCastRole(t *testing.T)
func TestResolveCastRoleNoSlot(t *testing.T)
func TestResolveCastRoleRefusal(t *testing.T)
func TestRoutedWorkshopSlotEditorRoles(t *testing.T)
func TestRoutedWorkshopSlotFailover(t *testing.T)
func TestRoutedWorkshopSlotNoKeys(t *testing.T)
func TestRoutedWorkshopSlotOneRequestOneModel(t *testing.T)
func TestRunHoroscopeForgeRating18UsesLocalSlot(t *testing.T)
func TestSchedulerOrder(t *testing.T)
func TestSedimentRun(t *testing.T)
func TestStyle(t *testing.T)
func TestWorldRating(t *testing.T)
func Validate(world *db.Handle, before, after string) (bool, string)
func Weight(p, luck float64) (float64)
func WorldRating(world *db.Handle) (string, error)
func WriteComplementCard(world *db.Handle, cardJSON string) (error)
interface Censor
interface DeferredClock
interface Lemmatizer
interface PauseClock
interface ResearchSource
interface RouteSlot
interface Slot
struct Arc
struct Book
struct BookMeta
struct Chapter
struct Chunk
struct ComplementPayload
struct ComposeOutput
struct DeferredTimer
struct EditorEdit
struct EditorQueuePayload
struct EditorQueueTask
struct EditorResult
struct EpochReference
struct ForgeBookOptions
struct ForgeBookPayload
struct ForgeBookResult
struct ForgeMaterial
struct GenerateOptions
struct HoroscopeBranch
struct HoroscopeMatch
struct HoroscopeMatchConfig
struct HoroscopeOutput
struct HoroscopePayload
struct LocalSource
struct Luck
struct Order
struct ParsedBook
struct PauseTimer
struct Preset
struct PythiaOutput
struct Roll
struct RoutedWorkshopSlot
struct Scheduler
struct SedimentOrderPayload
struct StyleConfig
struct StyleSample
struct Transcode
type DeferredStatus
type Format
type Kind
type PauseStep
type PromptFunc
type Provider
type ResearchSourceFunc
type SedimentOutcome
type SedimentRunFunc
type Status
type Step
var DefaultSedimentRun
var ErrBookNotFound
var ErrCensorReject
var ErrForgeDegraded
var ErrRemeltNotUnchecked
var ErrRouteNotWired
var ErrUnknownCharset
var ErrUnsupportedFormat
var Kinds
var MoralMarkers
```

## `_/home/ajaxii/PRJ/TNS65/engine/web`

```go
var FS
```

