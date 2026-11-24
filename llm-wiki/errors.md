# Error and Panic Sites

All indexed `errors.New`, `fmt.Errorf`, sentinel `var` declarations, and `panic` calls.

Total: 887 error sites.

| Message | Function | File |
|---------|----------|------|
| `status %d for %s` | `downloadTile` | Levant/generate_SVG_map.go:54 |
| `tns65 lang: missing command (list)` | `runLang` | engine/cmd/tns65/main.go:353 |
| `tns65 lang list: no arguments expected` | `runLang` | engine/cmd/tns65/main.go:358 |
| `tns65 lang: unknown command %q` | `runLang` | engine/cmd/tns65/main.go:362 |
| `tns65 world: missing command (birth, list)` | `runWorld` | engine/cmd/tns65/main.go:384 |
| `tns65 world list: no arguments expected` | `runWorld` | engine/cmd/tns65/main.go:391 |
| `tns65 world: unknown command %q` | `runWorld` | engine/cmd/tns65/main.go:395 |
| `tns65 book: missing command (export)` | `runBook` | engine/cmd/tns65/main.go:415 |
| `tns65 book: unknown command %q` | `runBook` | engine/cmd/tns65/main.go:421 |
| `tns65 book export: unexpected argument %q` | `runBookExport` | engine/cmd/tns65/main.go:437 |
| `tns65 book export: флаг --world обязателен` | `runBookExport` | engine/cmd/tns65/main.go:440 |
| `tns65 book export: %w` | `runBookExport` | engine/cmd/tns65/main.go:450 |
| `tns65 book export: %w` | `runBookExport` | engine/cmd/tns65/main.go:456 |
| `tns65 world birth: флаг --ancketa обязателен` | `runWorldBirth` | engine/cmd/tns65/main.go:476 |
| `tns65 world birth: %w` | `runWorldBirth` | engine/cmd/tns65/main.go:481 |
| `tns65 world birth: чтение анкеты: %w` | `runWorldBirth` | engine/cmd/tns65/main.go:485 |
| `tns65 tunnel: unexpected argument %q` | `runTunnel` | engine/cmd/tns65/main.go:520 |
| `tns65 tunnel: %w` | `runTunnel` | engine/cmd/tns65/main.go:525 |
| `tns65 tunnel: %w` | `runTunnel` | engine/cmd/tns65/main.go:528 |
| `tns65 tunnel: start scripts/tunnel.sh: %w` | `runTunnelScript` | engine/cmd/tns65/main.go:562 |
| `tns65 tunnel: scripts/tunnel.sh: %w` | `runTunnelScript` | engine/cmd/tns65/main.go:577 |
| `tns65 swarm: missing task (forge-card, deepread)` | `runSwarm` | engine/cmd/tns65/main.go:593 |
| `tns65 swarm: unknown task %q` | `runSwarm` | engine/cmd/tns65/main.go:601 |
| `tns65 swarm %s: expected <literary_db> <payload_json>` | `launchSwarmTask` | engine/cmd/tns65/main.go:610 |
| `bake: empty DB path` | `Bake` | engine/internal/bake/bake.go:59 |
| `bake: mkdir %s: %w` | `Bake` | engine/internal/bake/bake.go:71 |
| `bake: open writer: %w` | `Bake` | engine/internal/bake/bake.go:79 |
| `bake: migrate: %w` | `Bake` | engine/internal/bake/bake.go:83 |
| `bake: close writer: %w` | `Bake` | engine/internal/bake/bake.go:86 |
| `bake: open handle: %w` | `writeSeed` | engine/internal/bake/bake.go:100 |
| `bake: embed url: %w` | `writeSeed` | engine/internal/bake/bake.go:106 |
| `bake: insert narrative %q: %w` | `writeSeed` | engine/internal/bake/bake.go:115 |
| `bake: narrative %q id: %w` | `writeSeed` | engine/internal/bake/bake.go:119 |
| `bake: embed %q: %w` | `writeSeed` | engine/internal/bake/bake.go:124 |
| `bake: insert vector %q: %w` | `writeSeed` | engine/internal/bake/bake.go:129 |
| `bake: insert verse %q: %w` | `writeSeed` | engine/internal/bake/bake.go:136 |
| `bake: read narratives dir: %w` | `loadSeed` | engine/internal/bake/bake.go:152 |
| `bake: read verses.txt: %w` | `readVerses` | engine/internal/bake/bake.go:175 |
| `bake: read narrative %s: %w` | `parseNarrative` | engine/internal/bake/bake.go:192 |
| `bake: %s: bad verse ref %q` | `parseNarrative` | engine/internal/bake/bake.go:209 |
| `bake: %s: verse ref %d out of range 1..%d` | `parseNarrative` | engine/internal/bake/bake.go:212 |
| `bake: %s: exactly one name required, got %d` | `parseNarrative` | engine/internal/bake/bake.go:219 |
| `bake: %s: exactly one arc_summary required, got %d` | `parseNarrative` | engine/internal/bake/bake.go:222 |
| `embedder status %d` | `embed` | engine/internal/bake/bake.go:258 |
| `embedder returned dim %d, want %d` | `embed` | engine/internal/bake/bake.go:265 |
| `bake: open %s for checksum: %w` | `sha256File` | engine/internal/bake/bake.go:287 |
| `bake: checksum %s: %w` | `sha256File` | engine/internal/bake/bake.go:292 |
| `bank: выборка превысила бюджет: %v ...` | `(*Bank).Pick` | engine/internal/bank/bank.go:215 |
| `bank: выборка превысила бюджет: %v ...` | `(*Bank).PickScene` | engine/internal/bank/bank.go:272 |
| `birth: эмбеддинг синопсиса: %w` | `selectAnchor` | engine/internal/birth/anchor.go:73 |
| `birth: в базе языка base.<lang>.db нет ве�...` | `selectAnchor` | engine/internal/birth/anchor.go:82 |
| `birth: размерность вектора наррат...` | `selectAnchor` | engine/internal/birth/anchor.go:89 |
| `birth: открыть базу языка base.<lang>.db ...` | `readNarrativeVectors` | engine/internal/birth/anchor.go:104 |
| `birth: читать векторы нарративов: %w` | `readNarrativeVectors` | engine/internal/birth/anchor.go:110 |
| `birth: разбор вектора нарратива: %w` | `readNarrativeVectors` | engine/internal/birth/anchor.go:119 |
| `birth: вектор нарратива %d: длина %d ...` | `readNarrativeVectors` | engine/internal/birth/anchor.go:122 |
| `birth: открыть базу языка для чтен...` | `(*builder).materializeAnchor` | engine/internal/birth/anchormat.go:36 |
| `якорный нарратив %d не найден в �...` | `(*builder).materializeAnchor` | engine/internal/birth/anchormat.go:43 |
| `birth: читать нарратив-якорь: %w` | `(*builder).materializeAnchor` | engine/internal/birth/anchormat.go:45 |
| `birth: открыть мир для чтения якор...` | `readAnchorNarrativeID` | engine/internal/birth/anchormat.go:71 |
| `birth: читать якорь из world_meta: %w` | `readAnchorNarrativeID` | engine/internal/birth/anchormat.go:77 |
| `birth: значение якоря %q не число: %w` | `readAnchorNarrativeID` | engine/internal/birth/anchormat.go:81 |
| `birth: читать verses якоря: %w` | `anchorVerses` | engine/internal/birth/anchormat.go:91 |
| `birth: разбор verse якоря: %w` | `anchorVerses` | engine/internal/birth/anchormat.go:99 |
| `birth: читать patterns якоря: %w` | `anchorFamilies` | engine/internal/birth/anchormat.go:114 |
| `birth: разбор pattern якоря: %w` | `anchorFamilies` | engine/internal/birth/anchormat.go:122 |
| `birth: маршалинг anchor_name: %w` | `writeAnchorMeta` | engine/internal/birth/anchormat.go:137 |
| `birth: маршалинг anchor_summary: %w` | `writeAnchorMeta` | engine/internal/birth/anchormat.go:141 |
| `birth: маршалинг anchor_verses: %w` | `writeAnchorMeta` | engine/internal/birth/anchormat.go:145 |
| `birth: маршалинг anchor_families: %w` | `writeAnchorMeta` | engine/internal/birth/anchormat.go:149 |
| `%w: %q` | `Birth` | engine/internal/birth/birth.go:131 |
| `birth: убрать остаток каталога %s: %w` | `Birth` | engine/internal/birth/birth.go:136 |
| `birth: проверить каталог мира: %w` | `Birth` | engine/internal/birth/birth.go:139 |
| `birth: создать каталог мира: %w` | `Birth` | engine/internal/birth/birth.go:157 |
| `birth: метка состояния: %w` | `Birth` | engine/internal/birth/birth.go:161 |
| `birth: шаг %q упал (инъекция теста)` | `(*builder).run` | engine/internal/birth/birth.go:285 |
| `birth: шаг %q: %w` | `(*builder).run` | engine/internal/birth/birth.go:288 |
| `birth: вызовов слота прозы %d, ожид�...` | `checkNoProseAtBirth` | engine/internal/birth/birth.go:494 |
| `birth: открыть базу для гвардии пр...` | `checkNoProseAtBirth` | engine/internal/birth/birth.go:498 |
| `birth: проверить %s: %w` | `checkNoProseAtBirth` | engine/internal/birth/birth.go:504 |
| `birth: %s непуста: %d строк` | `checkNoProseAtBirth` | engine/internal/birth/birth.go:507 |
| `birth: маршалинг анкеты: %w` | `ancketaValues` | engine/internal/birth/birth.go:517 |
| `birth: разбор анкеты: %w` | `ancketaValues` | engine/internal/birth/birth.go:521 |
| `birth: ключ %q отсутствует в анкете` | `ancketaValues` | engine/internal/birth/birth.go:533 |
| `%w — заполните синопсис` | `withSynopsisHint` | engine/internal/birth/birth.go:557 |
| `slot down` | `TestBirthLocalPreset` | engine/internal/birth/birth_local_test.go:24 |
| `нет контрольной суммы базы %s` | `VerifyBaseChecksum` | engine/internal/birth/checksum.go:26 |
| `читать базу языка для контроль�...` | `VerifyBaseChecksum` | engine/internal/birth/checksum.go:35 |
| `читать базу языка для контроль�...` | `VerifyBaseChecksum` | engine/internal/birth/checksum.go:41 |
| `фикстурная база пуста` | `copyAndCorrupt` | engine/internal/birth/checksum_test.go:109 |
| `consumers: ключ %q без потребителя` | `CheckConsumers` | engine/internal/birth/consumers.go:58 |
| `consumers: у ключа %q пустой список п�...` | `CheckConsumers` | engine/internal/birth/consumers.go:61 |
| `consumers: ключ %q отсутствует в анке...` | `CheckConsumers` | engine/internal/birth/consumers.go:66 |
| `consumers: мировые поля %v, want %v` | `CheckConsumers` | engine/internal/birth/consumers.go:70 |
| `consumers: поля переигровки %v, want %v` | `CheckConsumers` | engine/internal/birth/consumers.go:73 |
| `разведка вернула статус %d` | `(*liveScoutSlot).Stream` | engine/internal/birth/deep_scout_test.go:42 |
| `birth: стартовый регион пуст` | `(*builder).geoSeed` | engine/internal/birth/geo_seed.go:67 |
| `birth: читать газетир %s: %w` | `(*builder).startRegionFromGazetteer` | engine/internal/birth/geo_seed.go:135 |
| `birth: газетир %s пуст` | `(*builder).startRegionFromGazetteer` | engine/internal/birth/geo_seed.go:138 |
| `%w: %q` | `replayBirth` | engine/internal/birth/isekai.go:42 |
| `birth: открыть мир для переигровк�...` | `replayBirth` | engine/internal/birth/isekai.go:48 |
| `birth: прочитать прошлого героя: %w` | `replayBirth` | engine/internal/birth/isekai.go:56 |
| `birth: переигровка: %w` | `replayBirth` | engine/internal/birth/isekai.go:68 |
| `birth: разбор hero_birth: %w` | `readHeroBirth` | engine/internal/birth/isekai.go:87 |
| `birth: сословий %d, want 6` | `Pack` | engine/internal/birth/pack.go:113 |
| `birth: таблица голосов пуста` | `Pack` | engine/internal/birth/pack.go:121 |
| `birth: таблица внешностей пуста` | `Pack` | engine/internal/birth/pack.go:129 |
| `birth: список архетипов пуст` | `Pack` | engine/internal/birth/pack.go:137 |
| `birth: код психотипа %q: нужно ровн�...` | `ParseCode` | engine/internal/birth/psychotype.go:39 |
| `birth: код психотипа %q: на позиции ...` | `ParseCode` | engine/internal/birth/psychotype.go:47 |
| `birth: полюс %q не принадлежит оси %s` | `axisValue` | engine/internal/birth/psychotype.go:89 |
| `birth: метрика %q ссылается на неиз...` | `ProfileFromCode` | engine/internal/birth/psychotype.go:138 |
| `birth: манифест пакета %s: %w` | `currentPackVersion` | engine/internal/birth/seed.go:133 |
| `birth: пакет %s без версии` | `currentPackVersion` | engine/internal/birth/seed.go:136 |
| `birth: %s: %w` | `readJSONLines` | engine/internal/birth/seed.go:186 |
| `birth: читать каталог миров: %w` | `List` | engine/internal/birth/worlds.go:36 |
| `birth: открыть базу мира %s: %w` | `readMetaTitle` | engine/internal/birth/worlds.go:81 |
| `birth: читать заголовок мира %s: %w` | `readMetaTitle` | engine/internal/birth/worlds.go:87 |
| `birth: разбор заголовка мира %s: %w` | `readMetaTitle` | engine/internal/birth/worlds.go:91 |
| `book: нет лемматизатора для распо...` | `DetectSleep` | engine/internal/book/chapter.go:157 |
| `book: нет лемматизатора для распо...` | `DetectClose` | engine/internal/book/chapter.go:174 |
| `book: нет хэндла мира` | `closeChapter` | engine/internal/book/chapter.go:211 |
| `book: id главы не задан` | `closeChapter` | engine/internal/book/chapter.go:214 |
| `book: id сессии главы не задан` | `closeChapter` | engine/internal/book/chapter.go:217 |
| `book: время закрытия не задано` | `closeChapter` | engine/internal/book/chapter.go:220 |
| `book: нет хэндла мира` | `AutoClose` | engine/internal/book/chapter.go:280 |
| `book: время автозакрытия не разоб...` | `AutoClose` | engine/internal/book/chapter.go:284 |
| `book: нет хэндла мира` | `(Chapter).Text` | engine/internal/book/chapter.go:504 |
| `book: id сессии главы не задан` | `(Chapter).Text` | engine/internal/book/chapter.go:507 |
| `слот недоступен` | `TestAutoCloseWorldNotesSlot` | engine/internal/book/chapter_test.go:496 |
| `%w: %d предложений, нужно %d–%d` | `ValidateClaim` | engine/internal/book/claim.go:71 |
| `book: нет хэндла мира` | `SaveClaim` | engine/internal/book/claim.go:89 |
| `book: эмбеддер не подключён` | `SaveClaim` | engine/internal/book/claim.go:92 |
| `book: статус заявки не задан` | `SaveClaim` | engine/internal/book/claim.go:98 |
| `book: нет хэндла мира` | `ProposeClaim` | engine/internal/book/claim.go:160 |
| `book: слот предложения заявки не �...` | `ProposeClaim` | engine/internal/book/claim.go:163 |
| `book: эмбеддер не подключён` | `ProposeClaim` | engine/internal/book/claim.go:166 |
| `book: нет хэндла мира` | `ApproveClaim` | engine/internal/book/claim.go:197 |
| `book: id заявки не задан` | `ApproveClaim` | engine/internal/book/claim.go:200 |
| `book: нет хэндла мира` | `CurrentClaim` | engine/internal/book/claim.go:224 |
| `book: нет хэндла мира` | `ClaimDrift` | engine/internal/book/claim.go:253 |
| `book: id заявки не задан` | `ClaimDrift` | engine/internal/book/claim.go:256 |
| `book: длина BLOB вектора %d не кратна 4` | `decodeVectorBlob` | engine/internal/book/claim.go:314 |
| `book: вектор заявки пуст` | `claimDrift` | engine/internal/book/claim.go:327 |
| `book: размерность вектора главы %d...` | `claimDrift` | engine/internal/book/claim.go:337 |
| `book: world_meta.synopsis не найден` | `claimSynopsis` | engine/internal/book/claim.go:386 |
| `book: разбор world_meta.synopsis: %w` | `claimSynopsis` | engine/internal/book/claim.go:393 |
| `book: нет хэндла мира` | `DeepRead` | engine/internal/book/deepread.go:145 |
| `book: id главы не задан` | `DeepRead` | engine/internal/book/deepread.go:148 |
| `book: реплика для разбора пуста` | `DeepRead` | engine/internal/book/deepread.go:151 |
| `book: слот разбора не подключён` | `DeepRead` | engine/internal/book/deepread.go:166 |
| `book: выход разбора не JSON: %w` | `DeepRead` | engine/internal/book/deepread.go:181 |
| `book: цитата поля %q не является по...` | `validateDeepRead` | engine/internal/book/deepread.go:237 |
| `%w: %d предложений, нужно %d–%d` | `ValidateDigest` | engine/internal/book/digest.go:60 |
| `book: нет хэндла мира` | `WriteDigest` | engine/internal/book/digest.go:99 |
| `book: id главы не задан` | `WriteDigest` | engine/internal/book/digest.go:102 |
| `book: нет хэндла мира` | `BuildDigest` | engine/internal/book/digest.go:126 |
| `book: id главы не задан` | `BuildDigest` | engine/internal/book/digest.go:129 |
| `book: слот Редактора не подключён` | `BuildDigest` | engine/internal/book/digest.go:132 |
| `book: нет хэндла мира` | `ApplyPlayerEdits` | engine/internal/book/edits.go:44 |
| `book: id главы не задан` | `ApplyPlayerEdits` | engine/internal/book/edits.go:47 |
| `book: нет хэндла мира` | `SaveEpilogue` | engine/internal/book/epilogue.go:47 |
| `book: заголовок эпилога не задан` | `SaveEpilogue` | engine/internal/book/epilogue.go:50 |
| `book: текст эпилога не задан` | `SaveEpilogue` | engine/internal/book/epilogue.go:53 |
| `book: нет хэндла мира` | `MarkPeak` | engine/internal/book/epilogue.go:88 |
| `book: id главы для метки не задан` | `MarkPeak` | engine/internal/book/epilogue.go:91 |
| `book: пометка пика не задана` | `MarkPeak` | engine/internal/book/epilogue.go:94 |
| `book: время метки не задано` | `MarkPeak` | engine/internal/book/epilogue.go:97 |
| `book: сцена метится по scene_id` | `MarkPeak` | engine/internal/book/epilogue.go:102 |
| `book: вид метки пика не задан` | `MarkPeak` | engine/internal/book/epilogue.go:107 |
| `book: нет хэндла мира` | `ScenePeaks` | engine/internal/book/epilogue.go:135 |
| `book: id сцены не задан` | `ScenePeaks` | engine/internal/book/epilogue.go:138 |
| `%w: %q` | `checkFormat` | engine/internal/book/export.go:35 |
| `book: world_meta.%s не найден` | `worldMetaString` | engine/internal/book/export.go:46 |
| `book: разбор world_meta.%s: %w` | `worldMetaString` | engine/internal/book/export.go:53 |
| `book: нет хэндла мира` | `queryBookChapters` | engine/internal/book/export.go:103 |
| `book: нет хэндла мира` | `ExportChapter` | engine/internal/book/export.go:133 |
| `book: номер главы не задан` | `ExportChapter` | engine/internal/book/export.go:136 |
| `book: нет хэндла мира` | `ExportBook` | engine/internal/book/export.go:177 |
| `book: нет хэндла мира` | `ExportReadyChapter` | engine/internal/book/export.go:324 |
| `book: id главы не задан` | `ExportReadyChapter` | engine/internal/book/export.go:327 |
| `book: глава не найдена` | `ExportReadyChapter` | engine/internal/book/export.go:335 |
| `book: нет хэндла мира` | `ExportReadyAll` | engine/internal/book/export.go:375 |
| `book: нет хэндла мира` | `AssembleChapter` | engine/internal/book/publish.go:34 |
| `book: id главы не задан` | `AssembleChapter` | engine/internal/book/publish.go:37 |
| `book: id сессии главы не задан` | `AssembleChapter` | engine/internal/book/publish.go:40 |
| `book: нет хэндла мира` | `Publish` | engine/internal/book/publish.go:71 |
| `book: id главы не задан` | `Publish` | engine/internal/book/publish.go:74 |
| `book: версия текста не задана` | `Publish` | engine/internal/book/publish.go:77 |
| `book: нет хэндла мира` | `LoadPublished` | engine/internal/book/publish.go:107 |
| `book: id главы не задан` | `LoadPublished` | engine/internal/book/publish.go:110 |
| `book: нет хэндла мира` | `Process` | engine/internal/book/publish_gate.go:96 |
| `book: id главы не задан` | `Process` | engine/internal/book/publish_gate.go:99 |
| `book: эмбеддер не подключён` | `Process` | engine/internal/book/publish_gate.go:102 |
| `book: глава не найдена` | `Process` | engine/internal/book/publish_gate.go:108 |
| `%w: %w` | `Process` | engine/internal/book/publish_gate.go:199 |
| `book: размерность вектора главы %d...` | `chapterVectorBlob` | engine/internal/book/publish_gate.go:208 |
| `book: нет хэндла мира` | `List` | engine/internal/book/toc.go:25 |
| `book: нет хэндла мира` | `OpenChapterText` | engine/internal/book/toc.go:54 |
| `book: id главы не задан` | `OpenChapterText` | engine/internal/book/toc.go:57 |
| `book: нет хэндла мира` | `EnsureSession` | engine/internal/book/wake.go:150 |
| `book: время прихода не задано` | `EnsureSession` | engine/internal/book/wake.go:153 |
| `book: нет хэндла мира` | `OpenChapter` | engine/internal/book/wake.go:178 |
| `book: день главы не задан` | `OpenChapter` | engine/internal/book/wake.go:181 |
| `book: нет хэндла мира` | `PrologueText` | engine/internal/book/wake.go:223 |
| `book: world_meta.prologue не найден` | `PrologueText` | engine/internal/book/wake.go:228 |
| `book: разбор world_meta.prologue: %w` | `PrologueText` | engine/internal/book/wake.go:234 |
| `book: нет хэндла мира` | `EnqueueWakeForge` | engine/internal/book/wake.go:244 |
| `book: id главы для пробуждения не з...` | `EnqueueWakeForge` | engine/internal/book/wake.go:247 |
| `book: нет хэндла мира` | `ForgeWakeup` | engine/internal/book/wake.go:266 |
| `book: id главы для пробуждения не з...` | `ForgeWakeup` | engine/internal/book/wake.go:269 |
| `book: время ковки не задано` | `ForgeWakeup` | engine/internal/book/wake.go:272 |
| `book: нет хэндла мира` | `LoadWakeup` | engine/internal/book/wake.go:288 |
| `book: id главы не задан` | `LoadWakeup` | engine/internal/book/wake.go:291 |
| `book: нет хэндла мира` | `DeliverWakeup` | engine/internal/book/wake.go:317 |
| `book: id главы не задан` | `DeliverWakeup` | engine/internal/book/wake.go:320 |
| `book: нужны часы для пассивного о�...` | `DeliverWakeup` | engine/internal/book/wake.go:328 |
| `book: ковка не готова и нет слота �...` | `serveWakeup` | engine/internal/book/wake.go:347 |
| `book: нет хэндла мира` | `OpenWindow` | engine/internal/book/window.go:81 |
| `book: неизвестная точка входа окна` | `OpenWindow` | engine/internal/book/window.go:84 |
| `cast: словарь описаний %s: %w` | `LoadDescriptions` | engine/internal/cast/describe.go:79 |
| `cast: словарь описаний %s: нет осей` | `LoadDescriptions` | engine/internal/cast/describe.go:82 |
| `cast: словарь описаний не задан` | `Describe` | engine/internal/cast/describe.go:118 |
| `cast: нет хэндла литературной базы` | `PromoteTopConfirmed` | engine/internal/cast/promote.go:26 |
| `cast: промоушен без книги` | `PromoteTopConfirmed` | engine/internal/cast/promote.go:29 |
| `cast: словарь ролей не задан` | `ChunkRoles` | engine/internal/cast/roles.go:154 |
| `слот отказал` | `TestRolesDisputedResolver` | engine/internal/cast/roles_test.go:150 |
| `cast: нет хэндла литературной базы` | `InsertChunk` | engine/internal/cast/store.go:74 |
| `cast: кусок без книги или хеша` | `InsertChunk` | engine/internal/cast/store.go:77 |
| `cast: нет хэндла литературной базы` | `InsertCharacter` | engine/internal/cast/store.go:108 |
| `cast: герой без книги или имени` | `InsertCharacter` | engine/internal/cast/store.go:111 |
| `cast: нет хэндла литературной базы` | `InsertMention` | engine/internal/cast/store.go:149 |
| `cast: упоминание без героя или ку�...` | `InsertMention` | engine/internal/cast/store.go:152 |
| `cast: нет хэндла литературной базы` | `InsertRole` | engine/internal/cast/store.go:163 |
| `cast: роль без героя или названия` | `InsertRole` | engine/internal/cast/store.go:166 |
| `лемматизатор недоступен` | `(errLemmatizer).Lemmatize` | engine/internal/censor/censor_test.go:130 |
| `db: bad migration filename %q: %w` | `Migrate` | engine/internal/db/migrate.go:58 |
| `db: нет хэндла для миграции` | `(*Handle).EnsureMigrated` | engine/internal/db/migrate.go:131 |
| `db: миграции базы %q не определены` | `(*Handle).EnsureMigrated` | engine/internal/db/migrate.go:135 |
| `deepread: режим походов %q вне списк�...` | `MapRole` | engine/internal/deepread/bookmap.go:67 |
| `deepread: карта книги: book_id %d` | `BuildBookMap` | engine/internal/deepread/bookmap.go:150 |
| `deepread: карта книги %d: %w` | `BuildBookMap` | engine/internal/deepread/bookmap.go:175 |
| `deepread: промпт карты книги %d: %w` | `mapPromptText` | engine/internal/deepread/bookmap.go:209 |
| `deepread: карта книги: book_id %d` | `LoadMapSource` | engine/internal/deepread/bookmap.go:221 |
| `deepread: места карты книги %d: %w` | `loadMapPlaces` | engine/internal/deepread/bookmap.go:251 |
| `deepread: места карты книги %d: %w` | `loadMapPlaces` | engine/internal/deepread/bookmap.go:264 |
| `deepread: алиасы места %q: %w` | `loadMapPlaces` | engine/internal/deepread/bookmap.go:275 |
| `deepread: места карты книги %d: %w` | `loadMapPlaces` | engine/internal/deepread/bookmap.go:282 |
| `deepread: сцены мест карты книги %d: %w` | `fillMapSceneCounts` | engine/internal/deepread/bookmap.go:293 |
| `deepread: сцены мест карты книги %d: %w` | `fillMapSceneCounts` | engine/internal/deepread/bookmap.go:303 |
| `deepread: сцены мест карты книги %d: %w` | `fillMapSceneCounts` | engine/internal/deepread/bookmap.go:310 |
| `deepread: сцены карты книги %d: %w` | `loadMapScenes` | engine/internal/deepread/bookmap.go:322 |
| `deepread: сцены карты книги %d: %w` | `loadMapScenes` | engine/internal/deepread/bookmap.go:336 |
| `deepread: участники сцены %d: %w` | `loadMapScenes` | engine/internal/deepread/bookmap.go:350 |
| `deepread: теги сцены %d: %w` | `loadMapScenes` | engine/internal/deepread/bookmap.go:353 |
| `deepread: сцены карты книги %d: %w` | `loadMapScenes` | engine/internal/deepread/bookmap.go:358 |
| `deepread: запись карты книги %d: %w` | `writeBookMap` | engine/internal/deepread/bookmap.go:376 |
| `deepread: запись карты книги %d: %w` | `writeBookMap` | engine/internal/deepread/bookmap.go:385 |
| `deepread: карта книги: book_id %d` | `LoadBookMap` | engine/internal/deepread/bookmap.go:397 |
| `deepread: карта книги %d: %w` | `LoadBookMap` | engine/internal/deepread/bookmap.go:403 |
| `deepread: карта книги %d: %w` | `LoadBookMap` | engine/internal/deepread/bookmap.go:410 |
| `deepread: карта книги %d: %w` | `LoadBookMap` | engine/internal/deepread/bookmap.go:421 |
| `providerroute: нет доступного провайд...` | `TestBuildBookMapDegradation` | engine/internal/deepread/bookmap_test.go:272 |
| `deepread: alias_cosine: %w` | `parseCharacterSection` | engine/internal/deepread/characters.go:90 |
| `deepread: alias_cosine %g вне 0..1` | `validateCharacterConfig` | engine/internal/deepread/characters.go:106 |
| `deepread: detectors_mood.json: %w` | `LoadEmotionDictionary` | engine/internal/deepread/characters.go:131 |
| `deepread: detectors_mood.json: пустая катего�...` | `LoadEmotionDictionary` | engine/internal/deepread/characters.go:136 |
| `deepread: detectors_mood.json: у категории %q �...` | `LoadEmotionDictionary` | engine/internal/deepread/characters.go:147 |
| `deepread: персонаж без id карты` | `MergeCharacters` | engine/internal/deepread/characters.go:284 |
| `deepread: персонаж %d повторяется` | `MergeCharacters` | engine/internal/deepread/characters.go:287 |
| `deepread: персонажи: book_id %d` | `LoadCharacterCandidates` | engine/internal/deepread/characters.go:562 |
| `deepread: алиасы героя %d: %w` | `parseAliases` | engine/internal/deepread/characters.go:627 |
| `deepread: векторы окон: book_id %d` | `AttachWindowVectors` | engine/internal/deepread/characters.go:641 |
| `deepread: вектор чанка %d: %w` | `chunkVectors` | engine/internal/deepread/characters.go:747 |
| `длина BLOB %d не кратна 4` | `vectorFromBlob` | engine/internal/deepread/characters.go:770 |
| `deepread: центроид: персонаж без id к...` | `WriteCharacterCentroids` | engine/internal/deepread/characters.go:840 |
| `deepread: центроид персонажа %d: dim %d,...` | `WriteCharacterCentroids` | engine/internal/deepread/characters.go:846 |
| `deepread: вектор персонажа %d: %w` | `WriteCharacterCentroids` | engine/internal/deepread/characters.go:853 |
| `deepread: вектор персонажа %d: %w` | `WriteCharacterCentroids` | engine/internal/deepread/characters.go:859 |
| `deepread: профиль: персонаж без id ка...` | `WriteCharacterProfiles` | engine/internal/deepread/characters.go:906 |
| `deepread: алиасы персонажа %d: %w` | `WriteCharacterProfiles` | engine/internal/deepread/characters.go:913 |
| `deepread: профиль: персонаж %d не най...` | `WriteCharacterProfiles` | engine/internal/deepread/characters.go:921 |
| `deepread: профиль эмоций персонажа ...` | `WriteCharacterProfiles` | engine/internal/deepread/characters.go:931 |
| `deepread: профиль персонажа %d: %w` | `WriteCharacterProfiles` | engine/internal/deepread/characters.go:939 |
| `deepread: у ключа %q нет файла словаря` | `LoadDictionary` | engine/internal/deepread/dicts.go:205 |
| `deepread: %s: %w` | `LoadDictionary` | engine/internal/deepread/dicts.go:213 |
| `deepread: %s: словарь пуст` | `LoadDictionary` | engine/internal/deepread/dicts.go:216 |
| `deepread: %s: пустая лемма` | `LoadDictionary` | engine/internal/deepread/dicts.go:223 |
| `deepread: %s: лемма %q: вес %d вне %d..%d` | `LoadDictionary` | engine/internal/deepread/dicts.go:226 |
| `deepread: %s: лемма %q повторяется` | `LoadDictionary` | engine/internal/deepread/dicts.go:230 |
| `deepread: %s: часть речи задана для л�...` | `LoadDictionary` | engine/internal/deepread/dicts.go:239 |
| `deepread: %s: лемма %q: часть речи %q вн�...` | `LoadDictionary` | engine/internal/deepread/dicts.go:243 |
| `deepread: %s: severity задана для леммы %q ...` | `LoadDictionary` | engine/internal/deepread/dicts.go:256 |
| `deepread: %s: лемма %q: severity %d вне %d..%d` | `LoadDictionary` | engine/internal/deepread/dicts.go:260 |
| `deepread: %s: лемма %q: вес %d не соотве�...` | `LoadDictionary` | engine/internal/deepread/dicts.go:264 |
| `deepread: %s: слой severity покрывает %d л�...` | `LoadDictionary` | engine/internal/deepread/dicts.go:270 |
| `deepread: styles.json: %w` | `LoadStyles` | engine/internal/deepread/dicts.go:301 |
| `deepread: styles.json: канон ярлыков пуст` | `LoadStyles` | engine/internal/deepread/dicts.go:304 |
| `deepread: styles.json: ярлык без имени` | `LoadStyles` | engine/internal/deepread/dicts.go:314 |
| `deepread: styles.json: ярлык %q повторяется` | `LoadStyles` | engine/internal/deepread/dicts.go:317 |
| `deepread: styles.json: ярлык %q: признак %q �...` | `LoadStyles` | engine/internal/deepread/dicts.go:322 |
| `deepread: styles.json: ярлык %q: признак %q �...` | `LoadStyles` | engine/internal/deepread/dicts.go:328 |
| `deepread: styles.json: ярлык %q: признак %q: ...` | `LoadStyles` | engine/internal/deepread/dicts.go:331 |
| `deepread: slot_budget %d < 0` | `validateDisputeConfig` | engine/internal/deepread/dispute.go:153 |
| `deepread: slot_batch %d < 1` | `validateDisputeConfig` | engine/internal/deepread/dispute.go:156 |
| `deepread: спор категории: chunk_id %d` | `BuildDisputes` | engine/internal/deepread/dispute.go:240 |
| `deepread: спор слияния: карточки %d/%d` | `BuildDisputes` | engine/internal/deepread/dispute.go:261 |
| `deepread: спор границы: %d→%d` | `BuildDisputes` | engine/internal/deepread/dispute.go:279 |
| `deepread: спор роли: персонаж %d, отр�...` | `BuildDisputes` | engine/internal/deepread/dispute.go:296 |
| `deepread: пустой ответ слота на спо�...` | `parseDisputeAnswers` | engine/internal/deepread/dispute.go:462 |
| `deepread: ответ слота не JSON-массив: %w` | `parseDisputeAnswers` | engine/internal/deepread/dispute.go:468 |
| `deepread: ответ слота не JSON: %w` | `parseDisputeAnswers` | engine/internal/deepread/dispute.go:473 |
| `deepread: ответов %d, вопросов %d` | `parseDisputeAnswers` | engine/internal/deepread/dispute.go:478 |
| `deepread: вопрос %q: значение %q вне к�...` | `parseDisputeAnswers` | engine/internal/deepread/dispute.go:486 |
| `deepread: вопрос %q: confidence %g вне 0..1` | `parseDisputeAnswers` | engine/internal/deepread/dispute.go:489 |
| `deepread: вопрос %q: цитата %q не явля�...` | `parseDisputeAnswers` | engine/internal/deepread/dispute.go:492 |
| `deepread: решение роли: персонаж %d, �...` | `ApplyDecision` | engine/internal/deepread/dispute.go:542 |
| `deepread: роль персонажа %d: %w` | `ApplyDecision` | engine/internal/deepread/dispute.go:552 |
| `deepread: слияние: карточки %d/%d без �...` | `applyMerge` | engine/internal/deepread/dispute.go:572 |
| `deepread: слияние: персонаж %d не най...` | `applyMerge` | engine/internal/deepread/dispute.go:578 |
| `deepread: слияние: алиасы персонажа...` | `applyMerge` | engine/internal/deepread/dispute.go:581 |
| `deepread: слияние: алиасы персонажа...` | `applyMerge` | engine/internal/deepread/dispute.go:585 |
| `deepread: слияние: алиасы персонажа...` | `applyMerge` | engine/internal/deepread/dispute.go:590 |
| `deepread: слияние: персонаж %d: %w` | `applyMerge` | engine/internal/deepread/dispute.go:595 |
| `deepread: слияние: персонаж %d: %w` | `applyMerge` | engine/internal/deepread/dispute.go:599 |
| `deepread: границы: нет набора сцен` | `ApplyBoundaries` | engine/internal/deepread/dispute.go:622 |
| `deepread: граница %d→%d не найдена ср...` | `joinScenes` | engine/internal/deepread/dispute.go:664 |
| `чанк %d: границы абзацев %d..%d вне ...` | `goldenChunkTexts` | engine/internal/deepread/golden_test.go:360 |
| `чанк %d: слов %d, в разметке %d` | `goldenChunkTexts` | engine/internal/deepread/golden_test.go:366 |
| `тегов %d, чанков разметки %d` | `scoreGolden` | engine/internal/deepread/golden_test.go:389 |
| `чанк %d, ключ %s: полоса %q вне confiden...` | `scoreGolden` | engine/internal/deepread/golden_test.go:446 |
| `deepread: версия словарей: %w` | `DictVersion` | engine/internal/deepread/key.go:62 |
| `deepread: версия словарей: %w` | `DictVersion` | engine/internal/deepread/key.go:68 |
| `deepread: модель эмбеддинга: %w` | `NewKey` | engine/internal/deepread/key.go:136 |
| `deepread: ключ разбора неполон: %s` | `FindRun` | engine/internal/deepread/key.go:164 |
| `deepread: журнал разбора: %w` | `FindRun` | engine/internal/deepread/key.go:171 |
| `deepread: журнал разбора: %w` | `FindRun` | engine/internal/deepread/key.go:178 |
| `deepread: журнал разбора: %w` | `FindRun` | engine/internal/deepread/key.go:189 |
| `deepread: ключ разбора неполон: %s` | `WriteRunLog` | engine/internal/deepread/key.go:201 |
| `deepread: число чанков разбора %d` | `WriteRunLog` | engine/internal/deepread/key.go:204 |
| `deepread: запись журнала разбора: %w` | `WriteRunLog` | engine/internal/deepread/key.go:211 |
| `deepread: прогон разбора без функции` | `RunOnce` | engine/internal/deepread/key.go:228 |
| `обрыв разбора` | `TestDeepreadKey` | engine/internal/deepread/key_test.go:280 |
| `deepread: лемматизация строки %d: %w` | `(*Run).lemmatizeText` | engine/internal/deepread/lemmas.go:126 |
| `deepread: пороги рейтинга вне 0..1: rati...` | `validateRatingConfig` | engine/internal/deepread/run.go:169 |
| `deepread: разбор книги: book_id %d` | `RunPipeline` | engine/internal/deepread/run.go:184 |
| `deepread: разбор книги %d: нет чанков` | `RunPipeline` | engine/internal/deepread/run.go:187 |
| `deepread: лемма-слой: %w` | `runLayers` | engine/internal/deepread/run.go:291 |
| `deepread: лемма-слой: %w` | `runLayers` | engine/internal/deepread/run.go:294 |
| `deepread: разбор книги %d: чанк %d без ...` | `resolveChunks` | engine/internal/deepread/run.go:488 |
| `deepread: разбор книги %d: ord %d` | `resolveChunks` | engine/internal/deepread/run.go:491 |
| `deepread: строка чанка %d/%d: %w` | `chunkRowID` | engine/internal/deepread/run.go:517 |
| `deepread: строка чанка %d/%d: %w` | `chunkRowID` | engine/internal/deepread/run.go:523 |
| `deepread: строка чанка %d/%d: %w` | `chunkRowID` | engine/internal/deepread/run.go:527 |
| `deepread: пак разбора %s: %w` | `resolveRunPaths` | engine/internal/deepread/run.go:556 |
| `deepread: сводка: чанки: %w` | `summarize` | engine/internal/deepread/run.go:722 |
| `deepread: сводка: сцены: %w` | `summarize` | engine/internal/deepread/run.go:725 |
| `deepread: сводка: персонажи: %w` | `summarize` | engine/internal/deepread/run.go:728 |
| `deepread: сводка: staging: %w` | `summarize` | engine/internal/deepread/run.go:731 |
| `deepread: сводка: полосы: %w` | `summarize` | engine/internal/deepread/run.go:738 |
| `deepread: сводка: полосы: %w` | `summarize` | engine/internal/deepread/run.go:745 |
| `deepread: сводка: полосы: %w` | `summarize` | engine/internal/deepread/run.go:755 |
| `deepread: сводка разбора: %w` | `writeSummary` | engine/internal/deepread/run.go:790 |
| `deepread: senses_margin %q: %v` | `parseSalienceSection` | engine/internal/deepread/salience.go:84 |
| `deepread: пороги salience: NaN` | `validateSalienceConfig` | engine/internal/deepread/salience.go:123 |
| `deepread: senses_margin %g: нужно 0 ≤ senses_margi...` | `validateSalienceConfig` | engine/internal/deepread/salience.go:126 |
| `deepread: пороги salience: confident %g, reject %g:...` | `validateSalienceConfig` | engine/internal/deepread/salience.go:129 |
| `deepread: нет словарей категорий дл...` | `TagChunksWithSenses` | engine/internal/deepread/salience.go:208 |
| `deepread: словарь ключа %q не загружен` | `TagChunksWithSenses` | engine/internal/deepread/salience.go:214 |
| `deepread: detectors_time.json: %w` | `LoadTimeLexicon` | engine/internal/deepread/scenes.go:62 |
| `deepread: detectors_time.json: нет единиц вре...` | `LoadTimeLexicon` | engine/internal/deepread/scenes.go:76 |
| `deepread: сцены: book_id %d` | `BuildScenes` | engine/internal/deepread/scenes.go:201 |
| `deepread: сцены: нет словаря времен�...` | `BuildScenes` | engine/internal/deepread/scenes.go:204 |
| `deepread: сцены: нет словаря места %q` | `BuildScenes` | engine/internal/deepread/scenes.go:208 |
| `deepread: сцены: чанк без id` | `BuildScenes` | engine/internal/deepread/scenes.go:216 |
| `deepread: сцены: чанк %d повторяется` | `BuildScenes` | engine/internal/deepread/scenes.go:219 |
| `deepread: запись сцен: нет книги` | `WriteScenes` | engine/internal/deepread/scenes.go:658 |
| `deepread: сцены книги %d: %w` | `WriteScenes` | engine/internal/deepread/scenes.go:676 |
| `deepread: места книги %d: %w` | `WriteScenes` | engine/internal/deepread/scenes.go:680 |
| `deepread: алиасы места %q: %w` | `WriteScenes` | engine/internal/deepread/scenes.go:687 |
| `deepread: место %q: %w` | `WriteScenes` | engine/internal/deepread/scenes.go:693 |
| `deepread: место %q: %w` | `WriteScenes` | engine/internal/deepread/scenes.go:697 |
| `deepread: сцена %d: место %q не найдено` | `WriteScenes` | engine/internal/deepread/scenes.go:709 |
| `deepread: сцена %d: участники: %w` | `WriteScenes` | engine/internal/deepread/scenes.go:716 |
| `deepread: сцена %d: теги: %w` | `WriteScenes` | engine/internal/deepread/scenes.go:720 |
| `deepread: сцена %d: эмоции: %w` | `WriteScenes` | engine/internal/deepread/scenes.go:728 |
| `deepread: сцена %d: %w` | `WriteScenes` | engine/internal/deepread/scenes.go:735 |
| `deepread: сцена %d: %w` | `WriteScenes` | engine/internal/deepread/scenes.go:739 |
| `deepread: вектор сцены %d: %w` | `WriteScenes` | engine/internal/deepread/scenes.go:753 |
| `deepread: центроид сцены %d: dim %d, want %d` | `WriteScenes` | engine/internal/deepread/scenes.go:761 |
| `deepread: вектор сцены %d: %w` | `WriteScenes` | engine/internal/deepread/scenes.go:768 |
| `deepread: место без ключа или имени` | `validateScenes` | engine/internal/deepread/scenes.go:780 |
| `deepread: место %q повторяется` | `validateScenes` | engine/internal/deepread/scenes.go:783 |
| `deepread: место %q: диапазон чанков %d...` | `validateScenes` | engine/internal/deepread/scenes.go:787 |
| `deepread: сцена %d: ord %d` | `validateScenes` | engine/internal/deepread/scenes.go:793 |
| `deepread: сцена %d: диапазон чанков %d...` | `validateScenes` | engine/internal/deepread/scenes.go:796 |
| `deepread: сцены %d и %d пересекаются п...` | `validateScenes` | engine/internal/deepread/scenes.go:800 |
| `deepread: сцена %d: место %q не найдено` | `validateScenes` | engine/internal/deepread/scenes.go:804 |
| `deepread: смыслы пака: %s: %w` | `LoadSenseCatalog` | engine/internal/deepread/senses_dispute.go:108 |
| `deepread: смыслы пака: %s: версия схе�...` | `LoadSenseCatalog` | engine/internal/deepread/senses_dispute.go:111 |
| `deepread: спорный смысл: chunk_id %d` | `BuildSenseDisputes` | engine/internal/deepread/senses_dispute.go:170 |
| `deepread: пустой ответ слота на спо�...` | `parseSenseAnswers` | engine/internal/deepread/senses_dispute.go:470 |
| `deepread: ответ слота не JSON-массив: %w` | `parseSenseAnswers` | engine/internal/deepread/senses_dispute.go:476 |
| `deepread: ответ слота не JSON: %w` | `parseSenseAnswers` | engine/internal/deepread/senses_dispute.go:481 |
| `deepread: ответов %d, вопросов %d` | `parseSenseAnswers` | engine/internal/deepread/senses_dispute.go:486 |
| `deepread: вопрос %q: смысл %q вне канона` | `parseSenseAnswers` | engine/internal/deepread/senses_dispute.go:494 |
| `deepread: вопрос %q: confidence %g вне 0..1` | `parseSenseAnswers` | engine/internal/deepread/senses_dispute.go:497 |
| `deepread: вопрос %q: цитата %q не явля�...` | `parseSenseAnswers` | engine/internal/deepread/senses_dispute.go:500 |
| `deepread: журнал спорных смыслов: %w` | `findSenseDisputes` | engine/internal/deepread/senses_dispute.go:599 |
| `deepread: журнал спорных смыслов: %w` | `findSenseDisputes` | engine/internal/deepread/senses_dispute.go:606 |
| `deepread: журнал спорных смыслов: %w` | `findSenseDisputes` | engine/internal/deepread/senses_dispute.go:617 |
| `deepread: запись спорных смыслов: %w` | `writeSenseDisputes` | engine/internal/deepread/senses_dispute.go:627 |
| `deepread: staging_cosine: %w` | `parseStagingSection` | engine/internal/deepread/staging.go:98 |
| `deepread: staging_cosine %g вне 0..1` | `validateStagingConfig` | engine/internal/deepread/staging.go:114 |
| `deepread: якорь %q: owner_id %d вне реестр�...` | `WriteDictAnchors` | engine/internal/deepread/staging.go:373 |
| `deepread: якорь %q: dim %d, want %d` | `WriteDictAnchors` | engine/internal/deepread/staging.go:376 |
| `deepread: якорь %q: %w` | `WriteDictAnchors` | engine/internal/deepread/staging.go:383 |
| `deepread: якорь %q: %w` | `WriteDictAnchors` | engine/internal/deepread/staging.go:389 |
| `deepread: staging: book_id %d` | `WriteStagingCandidates` | engine/internal/deepread/staging.go:412 |
| `deepread: staging: ключ %q вне реестра ка...` | `WriteStagingCandidates` | engine/internal/deepread/staging.go:423 |
| `deepread: staging: пустая лемма для клю�...` | `WriteStagingCandidates` | engine/internal/deepread/staging.go:426 |
| `deepread: staging: лемма %q: вес %d вне %d..%d` | `WriteStagingCandidates` | engine/internal/deepread/staging.go:429 |
| `deepread: staging: лемма %q: нет основани...` | `WriteStagingCandidates` | engine/internal/deepread/staging.go:433 |
| `deepread: staging: кандидат (%q, %q) повтор...` | `WriteStagingCandidates` | engine/internal/deepread/staging.go:438 |
| `deepread: staging: лемма %q: %w` | `WriteStagingCandidates` | engine/internal/deepread/staging.go:447 |
| `deepread: staging: лемма %q: %w` | `WriteStagingCandidates` | engine/internal/deepread/staging.go:451 |
| `deepread: staging: лемма %q: %w` | `WriteStagingCandidates` | engine/internal/deepread/staging.go:461 |
| `deepread: staging: book_id %d` | `StageDictionaries` | engine/internal/deepread/staging.go:481 |
| `deepread: staging: нет словарей категор...` | `StageDictionaries` | engine/internal/deepread/staging.go:491 |
| `deepread: тег чанка: chunk_id %d` | `WriteChunkTags` | engine/internal/deepread/store.go:26 |
| `deepread: теги чанка %d: %w` | `WriteChunkTags` | engine/internal/deepread/store.go:30 |
| `deepread: тег чанка %d: пустой ключ` | `WriteChunkTags` | engine/internal/deepread/store.go:36 |
| `deepread: тег чанка %d: ключ %q повтор�...` | `WriteChunkTags` | engine/internal/deepread/store.go:39 |
| `deepread: тег чанка %d: ключ %q: полоса...` | `WriteChunkTags` | engine/internal/deepread/store.go:43 |
| `deepread: тег чанка %d, ключ %q: %w` | `WriteChunkTags` | engine/internal/deepread/store.go:53 |
| `deepread: якоря тега: %w` | `marshalAnchors` | engine/internal/deepread/store.go:78 |
| `deepread: стиль книги: канон styles.json �...` | `BuildBookStyle` | engine/internal/deepread/style.go:95 |
| `deepread: стиль книги: нет лемм прог...` | `BuildBookStyle` | engine/internal/deepread/style.go:98 |
| `deepread: стиль книги: в тексте нет �...` | `BuildBookStyle` | engine/internal/deepread/style.go:102 |
| `deepread: styles.json: ярлык %q вне канона` | `styleLabelsFor` | engine/internal/deepread/style.go:344 |
| `deepread: запись стиля: нет книги` | `WriteBookStyle` | engine/internal/deepread/style.go:377 |
| `deepread: ярлыки стиля: %w` | `WriteBookStyle` | engine/internal/deepread/style.go:393 |
| `deepread: драматичность стиля: %w` | `WriteBookStyle` | engine/internal/deepread/style.go:399 |
| `deepread: стиль книги %d: %w` | `WriteBookStyle` | engine/internal/deepread/style.go:414 |
| `deepread: признаки стиля: %w` | `styleFeaturesJSON` | engine/internal/deepread/style.go:439 |
| `deepread: стиль книги: профиль не за...` | `validateBookStyle` | engine/internal/deepread/style.go:459 |
| `deepread: стиль книги: признак %q вне...` | `validateBookStyle` | engine/internal/deepread/style.go:463 |
| `deepread: стиль книги: признак %q = %g` | `validateBookStyle` | engine/internal/deepread/style.go:466 |
| `deepread: стиль книги: признак %q = %g �...` | `validateBookStyle` | engine/internal/deepread/style.go:469 |
| `deepread: стиль книги: ярлык %q вне к�...` | `validateBookStyle` | engine/internal/deepread/style.go:480 |
| `deepread: стиль книги: ярлык %q повто...` | `validateBookStyle` | engine/internal/deepread/style.go:483 |
| `deepread: стиль книги: корзина %q вне...` | `validateBookStyle` | engine/internal/deepread/style.go:497 |
| `deepread: стиль книги: корзина %q = %d` | `validateBookStyle` | engine/internal/deepread/style.go:500 |
| `deepread: стиль книги: частота част�...` | `validateBookStyle` | engine/internal/deepread/style.go:506 |
| `deepread: стиль книги: драматичност...` | `validateBookStyle` | engine/internal/deepread/style.go:513 |
| `deepread: стиль книги: чанк %d повтор...` | `validateBookStyle` | engine/internal/deepread/style.go:516 |
| `deepread: стиль книги: драматичност...` | `validateBookStyle` | engine/internal/deepread/style.go:520 |
| `embedserver: %w` | `checkHealth` | engine/internal/embedserver/embedserver.go:120 |
| `embedserver: bad base url %q` | `checkHealth` | engine/internal/embedserver/embedserver.go:124 |
| `embedserver: пулинг не тот: близкие ф�...` | `checkPooling` | engine/internal/embedserver/embedserver.go:174 |
| `geo: dayNormKm %v вне диапазона [%v, %v]` | `CheckDayNorm` | engine/internal/geo/days.go:34 |
| `geo: разбор world_meta.%s: %w` | `ReadDayNorm` | engine/internal/geo/days.go:83 |
| `geo: промпт содержит запретную п�...` | `CheckPrompt` | engine/internal/geo/days.go:106 |
| `модель недоступна` | `TestBuildTranslateDegrade` | engine/internal/langpack/langpack_test.go:287 |
| `langpack: invalid version %q` | `validateVersion` | engine/internal/langpack/versions.go:19 |
| `langpack: invalid version %q` | `validateVersion` | engine/internal/langpack/versions.go:23 |
| `langpack: invalid version %q` | `validateVersion` | engine/internal/langpack/versions.go:26 |
| `langpack: empty current pointer` | `currentVersion` | engine/internal/langpack/versions.go:66 |
| `langpack: rollback: %w` | `Rollback` | engine/internal/langpack/versions.go:98 |
| `langpack: rollback: no previous version` | `Rollback` | engine/internal/langpack/versions.go:109 |
| `langpack: rollback: verify %s: %w` | `Rollback` | engine/internal/langpack/versions.go:114 |
| `lemmarpc: bad request: %w` | `ReadRequest` | engine/internal/lemmarpc/protocol.go:62 |
| `lemmarpc: bad response: %w` | `ReadResponse` | engine/internal/lemmarpc/protocol.go:80 |
| `models: адрес эмбеддера: %w` | `NewClient` | engine/internal/models/embed.go:45 |
| `embedder status %d` | `(*Client).Embed` | engine/internal/models/embed.go:85 |
| `embedder returned dim %d, want %d` | `(*Client).Embed` | engine/internal/models/embed.go:96 |
| `models: загрузка: пустой url` | `DownloadFile` | engine/internal/models/fetch.go:107 |
| `models: загрузка: пустой путь назн�...` | `DownloadFile` | engine/internal/models/fetch.go:111 |
| `models: загрузка: url %q` | `DownloadFile` | engine/internal/models/fetch.go:115 |
| `models: загрузка: каталог назначен...` | `DownloadFile` | engine/internal/models/fetch.go:152 |
| `models: загрузка: каталог отрезков...` | `DownloadFile` | engine/internal/models/fetch.go:156 |
| `models: отрезок %d/%d: %w` | `DownloadFile` | engine/internal/models/fetch.go:186 |
| `models: загрузка: удаление отрезко...` | `DownloadFile` | engine/internal/models/fetch.go:205 |
| `models: загрузка: запрос пробы: %w` | `probeDownload` | engine/internal/models/fetch.go:232 |
| `models: загрузка: проба Range: %w` | `probeDownload` | engine/internal/models/fetch.go:237 |
| `models: загрузка: Content-Range %q` | `probeDownload` | engine/internal/models/fetch.go:246 |
| `models: загрузка: проба Range: статус %d` | `probeDownload` | engine/internal/models/fetch.go:255 |
| `models: формат Content-Range %q` | `parseContentRange` | engine/internal/models/fetch.go:262 |
| `models: формат Content-Range %q` | `parseContentRange` | engine/internal/models/fetch.go:268 |
| `models: формат Content-Range %q` | `parseContentRange` | engine/internal/models/fetch.go:272 |
| `models: формат Content-Range %q` | `parseContentRange` | engine/internal/models/fetch.go:276 |
| `models: формат Content-Range %q` | `parseContentRange` | engine/internal/models/fetch.go:282 |
| `models: частичный файл: %w` | `(*downloader).segment` | engine/internal/models/fetch.go:333 |
| `models: размер отрезка: %w` | `(*downloader).syncFile` | engine/internal/models/fetch.go:375 |
| `models: обрезка отрезка: %w` | `(*downloader).syncFile` | engine/internal/models/fetch.go:383 |
| `models: позиция отрезка: %w` | `(*downloader).syncFile` | engine/internal/models/fetch.go:386 |
| `models: запрос отрезка: %w` | `(*downloader).attempt` | engine/internal/models/fetch.go:395 |
| `models: запрос отрезка: %w` | `(*downloader).attempt` | engine/internal/models/fetch.go:409 |
| `models: отрезок: статус %d` | `(*downloader).attempt` | engine/internal/models/fetch.go:418 |
| `models: отрезок: %w` | `(*downloader).attempt` | engine/internal/models/fetch.go:427 |
| `models: отрезок: начало %d вместо %d` | `(*downloader).attempt` | engine/internal/models/fetch.go:430 |
| `models: отрезок: размер файла %d вме...` | `(*downloader).attempt` | engine/internal/models/fetch.go:433 |
| `models: чтение отрезка: %w` | `(*downloader).attempt` | engine/internal/models/fetch.go:446 |
| `models: отрезок оборван: %d из %d байт` | `(*downloader).attempt` | engine/internal/models/fetch.go:449 |
| `models: слияние: %w` | `mergeParts` | engine/internal/models/fetch.go:525 |
| `models: слияние: %w` | `mergeParts` | engine/internal/models/fetch.go:535 |
| `models: слияние: %w` | `mergeParts` | engine/internal/models/fetch.go:541 |
| `models: слияние: %w` | `mergeParts` | engine/internal/models/fetch.go:545 |
| `models: слияние: отрезок %d: %d из %d б�...` | `mergeParts` | engine/internal/models/fetch.go:549 |
| `models: слияние: %w` | `mergeParts` | engine/internal/models/fetch.go:555 |
| `models: слияние: %w` | `mergeParts` | engine/internal/models/fetch.go:558 |
| `models: адрес провайдера %q` | `NewOpenAIClient` | engine/internal/models/openai.go:42 |
| `%w: status %d` | `(*OpenAIClient).Stream` | engine/internal/models/openai.go:132 |
| `%w: status %d` | `(*OpenAIClient).Stream` | engine/internal/models/openai.go:134 |
| `models: адрес провайдера %q` | `ListOpenAIModels` | engine/internal/models/openai.go:207 |
| `%w: status %d` | `ListOpenAIModels` | engine/internal/models/openai.go:224 |
| `%w: list models: status %d` | `ListOpenAIModels` | engine/internal/models/openai.go:226 |
| `%w: %w` | `ListOpenAIModels` | engine/internal/models/openai.go:230 |
| `models: список моделей: %w` | `ListOpenAIModels` | engine/internal/models/openai.go:234 |
| `models: адрес провайдера: %w` | `NewProviderClient` | engine/internal/models/provider.go:192 |
| `%w: status %d` | `(*ProviderClient).Stream` | engine/internal/models/provider.go:293 |
| `%w: %w` | `classifyDoErr` | engine/internal/models/provider.go:327 |
| `conn refused` | `TestProviderContract` | engine/internal/models/provider_test.go:256 |
| `models: адрес слота прозы: %w` | `NewWeaver` | engine/internal/models/weaver.go:40 |
| `models: слот прозы status %d` | `(*Weaver).Stream` | engine/internal/models/weaver.go:87 |
| `paganel: хэндл мира не задан` | `BuildAtlasView` | engine/internal/paganel/atlas_view.go:77 |
| `paganel: читать places: %w` | `readAtlasPlaces` | engine/internal/paganel/atlas_view.go:125 |
| `paganel: разбор места: %w` | `readAtlasPlaces` | engine/internal/paganel/atlas_view.go:133 |
| `paganel: читать places: %w` | `readAtlasPlaces` | engine/internal/paganel/atlas_view.go:144 |
| `paganel: читать paths: %w` | `readAtlasPaths` | engine/internal/paganel/atlas_view.go:161 |
| `paganel: разбор пути: %w` | `readAtlasPaths` | engine/internal/paganel/atlas_view.go:170 |
| `paganel: читать paths: %w` | `readAtlasPaths` | engine/internal/paganel/atlas_view.go:186 |
| `paganel: читать rings: %w` | `readAtlasRings` | engine/internal/paganel/atlas_view.go:195 |
| `paganel: разбор кольца: %w` | `readAtlasRings` | engine/internal/paganel/atlas_view.go:203 |
| `paganel: читать rings: %w` | `readAtlasRings` | engine/internal/paganel/atlas_view.go:208 |
| `paganel: world_meta handle не задан` | `ReadTravelMode` | engine/internal/paganel/calibrate.go:44 |
| `paganel: world_meta.%s отсутствует` | `ReadTravelMode` | engine/internal/paganel/calibrate.go:49 |
| `paganel: читать world_meta.%s: %w` | `ReadTravelMode` | engine/internal/paganel/calibrate.go:52 |
| `paganel: разбор world_meta.%s: %w` | `ReadTravelMode` | engine/internal/paganel/calibrate.go:56 |
| `paganel: режим походов %q вне списка...` | `ReadTravelMode` | engine/internal/paganel/calibrate.go:60 |
| `paganel: фраза %q не называет время �...` | `FictionDistance` | engine/internal/paganel/calibrate.go:89 |
| `paganel: пустая фраза о пути` | `parseNamedTravel` | engine/internal/paganel/calibrate.go:125 |
| `paganel: фраза %q не разобрана как в�...` | `parseNamedTravel` | engine/internal/paganel/calibrate.go:141 |
| `paganel: хэндл ядра не задан` | `RecordCorridor` | engine/internal/paganel/corridor.go:90 |
| `paganel: опорные точки коридора: %w` | `RecordCorridor` | engine/internal/paganel/corridor.go:98 |
| `paganel: остановки коридора: %w` | `RecordCorridor` | engine/internal/paganel/corridor.go:102 |
| `paganel: записать коридор: %w` | `RecordCorridor` | engine/internal/paganel/corridor.go:116 |
| `paganel: id коридора: %w` | `RecordCorridor` | engine/internal/paganel/corridor.go:120 |
| `paganel: остановка коридора %q: %w` | `RecordCorridor` | engine/internal/paganel/corridor.go:140 |
| `paganel: коридор без концов маршрута` | `validateCorridorPlan` | engine/internal/paganel/corridor.go:150 |
| `paganel: коридор без опорных точек` | `validateCorridorPlan` | engine/internal/paganel/corridor.go:153 |
| `paganel: опорная точка %q: день %v вне...` | `validateCorridorPlan` | engine/internal/paganel/corridor.go:157 |
| `paganel: остановка %q: день %v вне доп...` | `validateCorridorPlan` | engine/internal/paganel/corridor.go:162 |
| `paganel: заявленный срок %v вне допу...` | `validateCorridorPlan` | engine/internal/paganel/corridor.go:166 |
| `paganel: сумма дней маршрута %.2f бол...` | `validateCorridorPlan` | engine/internal/paganel/corridor.go:169 |
| `paganel: build_budget %q не число` | `parseBuildBudget` | engine/internal/paganel/corridor.go:308 |
| `paganel: build_budget %d вне допустимого` | `parseBuildBudget` | engine/internal/paganel/corridor.go:311 |
| `paganel: %w` | `NewDaysCalc` | engine/internal/paganel/days.go:23 |
| `paganel: протокол %q не поддерживае�...` | `newScoutSlot` | engine/internal/paganel/deep_scout.go:344 |
| `разведка вернула статус %d` | `(*liveSlot).Stream` | engine/internal/paganel/deep_scout_test.go:45 |
| `paganel: хэндл ковки не задан` | `ForgeRing` | engine/internal/paganel/forge_ring.go:161 |
| `paganel: слот ковки кольца не подкл...` | `ForgeRing` | engine/internal/paganel/forge_ring.go:165 |
| `paganel: выход ковки кольца не JSON: %w` | `ForgeRing` | engine/internal/paganel/forge_ring.go:182 |
| `paganel: место без имени в выходе к�...` | `validateForgeOutput` | engine/internal/paganel/forge_ring.go:199 |
| `paganel: место %q: distance_days = %v вне доп�...` | `validateForgeOutput` | engine/internal/paganel/forge_ring.go:205 |
| `paganel: путь без концов в выходе к�...` | `validateForgeOutput` | engine/internal/paganel/forge_ring.go:210 |
| `paganel: записать место %q: %w` | `writeDrafts` | engine/internal/paganel/forge_ring.go:249 |
| `paganel: id места %q: %w` | `writeDrafts` | engine/internal/paganel/forge_ring.go:253 |
| `paganel: записать путь %q→%q: %w` | `writeDrafts` | engine/internal/paganel/forge_ring.go:269 |
| `paganel: хэндл ковки не задан` | `PromoteDrafts` | engine/internal/paganel/forge_ring.go:283 |
| `paganel: черновики мест: %w` | `PromoteDrafts` | engine/internal/paganel/forge_ring.go:299 |
| `paganel: строка черновика места: %w` | `PromoteDrafts` | engine/internal/paganel/forge_ring.go:313 |
| `paganel: черновики мест: %w` | `PromoteDrafts` | engine/internal/paganel/forge_ring.go:320 |
| `paganel: продвинуть место %d: %w` | `PromoteDrafts` | engine/internal/paganel/forge_ring.go:335 |
| `paganel: продвинуть пути: %w` | `PromoteDrafts` | engine/internal/paganel/forge_ring.go:347 |
| `paganel: читать место %q: %w` | `placeIDByName` | engine/internal/paganel/forge_ring.go:384 |
| `paganel: направление %q вне списка (�...` | `parseBearing` | engine/internal/paganel/forge_ring.go:433 |
| `paganel: тип пути %q вне списка trakt|riv...` | `ValidPathType` | engine/internal/paganel/geomodel.go:39 |
| `paganel: открыть базу мира %s: %w` | `loadGeomodelFromPath` | engine/internal/paganel/geomodel.go:109 |
| `paganel: читать places: %w` | `readPlaces` | engine/internal/paganel/geomodel.go:127 |
| `paganel: разбор места: %w` | `readPlaces` | engine/internal/paganel/geomodel.go:137 |
| `paganel: читать places: %w` | `readPlaces` | engine/internal/paganel/geomodel.go:144 |
| `paganel: читать paths: %w` | `readPaths` | engine/internal/paganel/geomodel.go:153 |
| `paganel: разбор пути: %w` | `readPaths` | engine/internal/paganel/geomodel.go:162 |
| `paganel: путь %d: %w` | `readPaths` | engine/internal/paganel/geomodel.go:165 |
| `paganel: читать paths: %w` | `readPaths` | engine/internal/paganel/geomodel.go:173 |
| `paganel: хэндл ядра не задан` | `MarkPlaceKnown` | engine/internal/paganel/player_map.go:30 |
| `paganel: place_id не задан` | `MarkPlaceKnown` | engine/internal/paganel/player_map.go:33 |
| `paganel: записать player_map: %w` | `MarkPlaceKnown` | engine/internal/paganel/player_map.go:45 |
| `paganel: герой не у места` | `MarkHeroPlaceKnown` | engine/internal/paganel/player_map.go:59 |
| `paganel: читать player_map: %w` | `ReadPlayerMap` | engine/internal/paganel/player_map.go:74 |
| `paganel: разбор player_map: %w` | `ReadPlayerMap` | engine/internal/paganel/player_map.go:83 |
| `paganel: читать player_map: %w` | `ReadPlayerMap` | engine/internal/paganel/player_map.go:89 |
| `paganel: читать character_state: %w` | `ReadHeroPosition` | engine/internal/paganel/position.go:44 |
| `paganel: хэндл мира не задан` | `writeHeroPosition` | engine/internal/paganel/position.go:74 |
| `paganel: записать character_state: %w` | `writeHeroPosition` | engine/internal/paganel/position.go:82 |
| `paganel: хэндл мира не задан` | `readPlaceCoords` | engine/internal/paganel/position.go:90 |
| `paganel: место %d не найдено в places` | `readPlaceCoords` | engine/internal/paganel/position.go:97 |
| `paganel: читать places: %w` | `readPlaceCoords` | engine/internal/paganel/position.go:99 |
| `paganel: читать places: %w` | `readKnownPlaceCoords` | engine/internal/paganel/position.go:139 |
| `paganel: разбор places: %w` | `readKnownPlaceCoords` | engine/internal/paganel/position.go:146 |
| `paganel: хэндл мира не задан` | `NewPrefilter` | engine/internal/paganel/prefilter.go:23 |
| `paganel: читать places: %w` | `(*Prefilter).loadPlaces` | engine/internal/paganel/prefilter.go:39 |
| `paganel: разбор места: %w` | `(*Prefilter).loadPlaces` | engine/internal/paganel/prefilter.go:45 |
| `paganel: читать player_map: %w` | `(*Prefilter).loadPlayerMap` | engine/internal/paganel/prefilter.go:56 |
| `paganel: разбор player_map: %w` | `(*Prefilter).loadPlayerMap` | engine/internal/paganel/prefilter.go:62 |
| `paganel: хэндл мира не задан` | `CollectRingJournal` | engine/internal/paganel/ring_journal.go:46 |
| `paganel: журнал колец: %w` | `CollectRingJournal` | engine/internal/paganel/ring_journal.go:52 |
| `paganel: строка журнала колец: %w` | `CollectRingJournal` | engine/internal/paganel/ring_journal.go:61 |
| `paganel: журнал колец: %w` | `CollectRingJournal` | engine/internal/paganel/ring_journal.go:75 |
| `paganel: статистика мест: %w` | `readForgeStats` | engine/internal/paganel/ring_journal.go:96 |
| `paganel: статистика путей: %w` | `readForgeStats` | engine/internal/paganel/ring_journal.go:104 |
| `paganel: хэндл мира не задан` | `EnqueueRing` | engine/internal/paganel/rings.go:54 |
| `paganel: поставить кольцо: %w` | `EnqueueRing` | engine/internal/paganel/rings.go:61 |
| `paganel: id кольца: %w` | `EnqueueRing` | engine/internal/paganel/rings.go:65 |
| `paganel: хэндл мира не задан` | `StartRing` | engine/internal/paganel/rings.go:74 |
| `paganel: запустить кольцо: %w` | `StartRing` | engine/internal/paganel/rings.go:81 |
| `paganel: кольцо %d не в очереди (queued)` | `StartRing` | engine/internal/paganel/rings.go:84 |
| `paganel: хэндл мира не задан` | `FinishRing` | engine/internal/paganel/rings.go:93 |
| `paganel: завершить кольцо: %w` | `FinishRing` | engine/internal/paganel/rings.go:100 |
| `paganel: кольцо %d не в работе (running)` | `FinishRing` | engine/internal/paganel/rings.go:103 |
| `paganel: хэндл мира не задан` | `UpdateRingCounters` | engine/internal/paganel/rings.go:113 |
| `paganel: счётчики кольца: %w` | `UpdateRingCounters` | engine/internal/paganel/rings.go:118 |
| `paganel: хэндл мира не задан` | `ReadRing` | engine/internal/paganel/rings.go:126 |
| `paganel: кольцо %d не найдено` | `ReadRing` | engine/internal/paganel/rings.go:134 |
| `paganel: читать кольцо: %w` | `ReadRing` | engine/internal/paganel/rings.go:136 |
| `paganel: текущее кольцо: %w` | `CurrentRing` | engine/internal/paganel/rings.go:154 |
| `paganel: клиент разведки не подклю�...` | `(*Scout).Fetch` | engine/internal/paganel/scout.go:102 |
| `paganel: запрос разведки: %w` | `(*Scout).Fetch` | engine/internal/paganel/scout.go:112 |
| `paganel: разведка вернула статус %d` | `(*Scout).Fetch` | engine/internal/paganel/scout.go:133 |
| `paganel: режим походов %q вне списка...` | `ScoutTask` | engine/internal/paganel/scout_route.go:28 |
| `portgate: bad local_address` | `parseLocalAddr` | engine/internal/portgate/portgate_linux.go:86 |
| `portgate: bad ip length` | `parseLocalAddr` | engine/internal/portgate/portgate_linux.go:108 |
| `portgate: bad ip hex length` | `decodeHexIP` | engine/internal/portgate/portgate_linux.go:119 |
| `providerroute: провайдер %q не найден в...` | `slotClientFactory` | engine/internal/providerroute/router.go:114 |
| `providerroute: провайдер %q не отдаёт м...` | `slotClientFactory` | engine/internal/providerroute/router.go:125 |
| `нет подставного клиента %q` | `testRouter` | engine/internal/providerroute/router_test.go:85 |
| `%w: status 500` | `TestRouterFailover` | engine/internal/providerroute/router_test.go:203 |
| `%w: status 429` | `TestRouterQuarantine429` | engine/internal/providerroute/router_test.go:238 |
| `%w: status 401` | `TestRouterKeyRejectedUntilRestart` | engine/internal/providerroute/router_test.go:315 |
| `%w: status 502` | `TestRouterAllFail` | engine/internal/providerroute/router_test.go:355 |
| `%w: status 503` | `TestRouterAllFail` | engine/internal/providerroute/router_test.go:356 |
| `%w: status 400` | `TestRouterNoFailoverOn400` | engine/internal/providerroute/router_test.go:417 |
| `%w: задача %s` | `(*RoutedProseSlot).Stream` | engine/internal/providerroute/slot.go:36 |
| `%w: задача %s: %w` | `(*RoutedProseSlot).Stream` | engine/internal/providerroute/slot.go:71 |
| `sediment: лишние данные после JSON-ве�...` | `parseVerdict` | engine/internal/sediment/autocritic.go:201 |
| `sediment: неизвестный вердикт %q` | `parseVerdict` | engine/internal/sediment/autocritic.go:207 |
| `заготовок больше нет` | `(*fakeAutocriticSlot).Generate` | engine/internal/sediment/autocritic_test.go:32 |
| `sediment: идемпотентность: %w` | `(*Pipeline).Run` | engine/internal/sediment/pipeline.go:68 |
| `%w: status=%q` | `(*Pipeline).Run` | engine/internal/sediment/pipeline.go:84 |
| `sediment: очистка: %w` | `(*Pipeline).Run` | engine/internal/sediment/pipeline.go:89 |
| `sediment: дедуп: %w` | `(*Pipeline).Run` | engine/internal/sediment/pipeline.go:97 |
| `sediment: дедуп: %w` | `(*Pipeline).Run` | engine/internal/sediment/pipeline.go:101 |
| `sediment: эмбеддер не задан` | `Vectorize` | engine/internal/sediment/vector.go:17 |
| `sediment: эмбеддинг очищенной запи�...` | `Vectorize` | engine/internal/sediment/vector.go:21 |
| `sediment: размерность вектора %d, want %d` | `Vectorize` | engine/internal/sediment/vector.go:24 |
| `sediment: размерность вектора %d, want %d` | `VectorBlob` | engine/internal/sediment/vector.go:32 |
| `sediment: длина BLOB %d не кратна 4` | `VectorFromBlob` | engine/internal/sediment/vector.go:44 |
| `%w: %s` | `Write` | engine/internal/sediment/write.go:45 |
| `senses: %s: %w` | `Load` | engine/internal/senses/senses.go:112 |
| `senses: %s: версия схемы %d, ожидалась 1` | `Load` | engine/internal/senses/senses.go:115 |
| `senses: %s: словарь смыслов пуст` | `Load` | engine/internal/senses/senses.go:118 |
| `senses: %s: смысл %d: пустая лемма` | `Load` | engine/internal/senses/senses.go:130 |
| `senses: %s: %s/%s: пустая часть речи` | `Load` | engine/internal/senses/senses.go:133 |
| `senses: %s: %s: пустое имя смысла` | `Load` | engine/internal/senses/senses.go:136 |
| `senses: %s: %s/%s: категория %q вне канона` | `Load` | engine/internal/senses/senses.go:139 |
| `senses: %s: %s/%s: триггеров %d, нужно не...` | `Load` | engine/internal/senses/senses.go:143 |
| `senses: %s: %s/%s: триггер без леммы` | `Load` | engine/internal/senses/senses.go:151 |
| `senses: %s: %s/%s: триггер %q: вес %g вне %g...` | `Load` | engine/internal/senses/senses.go:154 |
| `senses: %s: %s/%s: триггер %q повторяется` | `Load` | engine/internal/senses/senses.go:158 |
| `senses: %s: смысл %s/%s/%s повторяется` | `Load` | engine/internal/senses/senses.go:167 |
| `senses: контексты смыслов без слов...` | `LoadContexts` | engine/internal/senses/vectors.go:56 |
| `senses: %s: %w` | `LoadContexts` | engine/internal/senses/vectors.go:65 |
| `senses: %s: версия схемы %d, ожидалась 1` | `LoadContexts` | engine/internal/senses/vectors.go:68 |
| `senses: %s: контексты смыслов пусты` | `LoadContexts` | engine/internal/senses/vectors.go:71 |
| `senses: %s: контексты %d: пустые лемм�...` | `LoadContexts` | engine/internal/senses/vectors.go:80 |
| `senses: %s: %s/%s/%s: смысла нет в словаре` | `LoadContexts` | engine/internal/senses/vectors.go:83 |
| `senses: %s: контексты смысла %s/%s/%s по...` | `LoadContexts` | engine/internal/senses/vectors.go:87 |
| `senses: %s: %s/%s/%s: контекстов %d, нужно...` | `LoadContexts` | engine/internal/senses/vectors.go:91 |
| `senses: %s: %s/%s/%s: пустой контекст` | `LoadContexts` | engine/internal/senses/vectors.go:99 |
| `senses: %s: %s/%s/%s: контекст %q повторя�...` | `LoadContexts` | engine/internal/senses/vectors.go:102 |
| `senses: %s: %s/%s/%s: имя собственное в к...` | `LoadContexts` | engine/internal/senses/vectors.go:106 |
| `senses: %s: у смысла %s/%s нет контекст...` | `LoadContexts` | engine/internal/senses/vectors.go:118 |
| `эмбеддер недоступен` | `(*failEmbed).Embed` | engine/internal/senses/vectors_test.go:70 |
| `structural: family_primary не указан` | `(Composition).Validate` | engine/internal/structural/composition.go:60 |
| `structural: pattern_secondary %q совпадает с fa...` | `(Composition).Validate` | engine/internal/structural/composition.go:63 |
| `structural: техника %q вне списка` | `(Composition).Validate` | engine/internal/structural/composition.go:66 |
| `structural: моральная позиция %q вне �...` | `(Composition).Validate` | engine/internal/structural/composition.go:69 |
| `structural: нет хэндла мира` | `BumpUsage` | engine/internal/structural/drift.go:38 |
| `structural: область счётчика %q вне ('c...` | `BumpUsage` | engine/internal/structural/drift.go:41 |
| `structural: семейство счётчика не за...` | `BumpUsage` | engine/internal/structural/drift.go:44 |
| `structural: нет хэндла мира` | `Usage` | engine/internal/structural/drift.go:57 |
| `structural: область счётчика %q вне ('c...` | `Usage` | engine/internal/structural/drift.go:60 |
| `structural: нет хэндла мира` | `ResetScope` | engine/internal/structural/drift.go:87 |
| `structural: область счётчика %q вне ('c...` | `ResetScope` | engine/internal/structural/drift.go:90 |
| `structural: read %s: %w` | `LoadFamilies` | engine/internal/structural/family.go:37 |
| `structural: %s: %w` | `LoadFamilies` | engine/internal/structural/family.go:41 |
| `structural: семей %d, want 14` | `LoadFamilies` | engine/internal/structural/family.go:44 |
| `structural: запись %d: пустое имя` | `LoadFamilies` | engine/internal/structural/family.go:49 |
| `structural: запись %d: имя %q не соотве�...` | `LoadFamilies` | engine/internal/structural/family.go:52 |
| `structural: запись %d: резерв %q не явл�...` | `LoadFamilies` | engine/internal/structural/family.go:55 |
| `structural: запись %d: дубль имени %q` | `LoadFamilies` | engine/internal/structural/family.go:58 |
| `structural: семейство %q без ключевых...` | `LoadFamilies` | engine/internal/structural/family.go:62 |
| `structural: w_family %.3f превышает порог %...` | `parseCompositionSection` | engine/internal/structural/score.go:138 |
| `supervisor: health %s: status %d` | `CheckHealth` | engine/internal/supervisor/health.go:34 |
| `supervisor: port %d is already in use: %w` | `CheckPortFree` | engine/internal/supervisor/port.go:18 |
| `supervisor: child %q not found` | `(*Supervisor).SwitchModel` | engine/internal/supervisor/status.go:51 |
| `supervisor: switch model %q: %w (rolled back)` | `(*Supervisor).SwitchModel` | engine/internal/supervisor/status.go:80 |
| `supervisor: child %q not found` | `(*Supervisor).StopChild` | engine/internal/supervisor/status.go:104 |
| `supervisor: child %q: bad port %q` | `setField` | engine/internal/supervisor/supervisor.go:279 |
| `supervisor: child %q: bad ready_timeout %q` | `setField` | engine/internal/supervisor/supervisor.go:289 |
| `supervisor: child %q: bad poll_interval %q` | `setField` | engine/internal/supervisor/supervisor.go:295 |
| `boom` | `TestRestart` | engine/internal/supervisor/supervisor_test.go:184 |
| `not ready` | `TestNoShortReadyBudget` | engine/internal/supervisor/supervisor_test.go:240 |
| `not ready` | `TestSwitchModelRollback` | engine/internal/supervisor/supervisor_test.go:431 |
| `swarm: task %s: read handshake: %w` | `Launch` | engine/internal/swarm/launch.go:66 |
| `swarm: task %s: %w` | `Launch` | engine/internal/swarm/launch.go:88 |
| `swarm: task %s: read result: %w` | `Launch` | engine/internal/swarm/launch.go:90 |
| `swarm: task %s: %w` | `Launch` | engine/internal/swarm/launch.go:95 |
| `COUNT(vectors): %w` | `assertVectorsEmpty` | engine/internal/turn/cut_test.go:131 |
| `vectors непуста: %d строк, ожидалось 0` | `assertVectorsEmpty` | engine/internal/turn/cut_test.go:134 |
| `turn: открыть базу языка для пула ...` | `PickPoolName` | engine/internal/turn/namepool.go:31 |
| `turn: читать пул имён: %w` | `PickPoolName` | engine/internal/turn/namepool.go:37 |
| `turn: разбор имени пула: %w` | `PickPoolName` | engine/internal/turn/namepool.go:46 |
| `провайдер недоступен` | `TestNewsSceneDegradation` | engine/internal/turn/news_scene_test.go:130 |
| `провайдер недоступен` | `TestSilenceThreshold` | engine/internal/turn/safety_test.go:99 |
| `turn: неизвестный протокол прова�...` | `ProviderSlot` | engine/internal/turn/slots.go:246 |
| `turn: нет хэндла мира` | `ChooseFinale` | engine/internal/turn/stages.go:806 |
| `turn: открытый финал не ждёт выбора` | `ChooseFinale` | engine/internal/turn/stages.go:813 |
| `turn: неизвестный путь открытого ...` | `ChooseFinale` | engine/internal/turn/stages.go:837 |
| `turn: инъекция отказа на таблице t...` | `commitTurn` | engine/internal/turn/stopcrane.go:48 |
| `turn: инъекция отказа на таблице s...` | `commitTurn` | engine/internal/turn/stopcrane.go:56 |
| `turn: инъекция отказа на таблице e...` | `commitTurn` | engine/internal/turn/stopcrane.go:63 |
| `turn: инъекция отказа на таблице v...` | `commitTurn` | engine/internal/turn/stopcrane.go:70 |
| `turn: инъекция отказа на таблице t...` | `commitTurn` | engine/internal/turn/stopcrane.go:77 |
| `turn: инъекция отказа на таблице c...` | `commitTurn` | engine/internal/turn/stopcrane.go:84 |
| `turn: вектор хода dim=%d, want %d` | `insertVector` | engine/internal/turn/stopcrane.go:172 |
| `отказ` | `TestWeaverCallJournal` | engine/internal/turn/stream_test.go:244 |
| `turn: карточка лица %d: %w` | `(*Troupe).Spawn` | engine/internal/turn/troupe.go:296 |
| `turn: карточка лица %d: %w` | `(*Troupe).Spawn` | engine/internal/turn/troupe.go:300 |
| `webapi: пачка пуста (нет кодов псих...` | `promoteFanficCast` | engine/internal/webapi/handlers_atlas.go:394 |
| `restart boom` | `TestChannelPutRestartErrorWarning` | engine/internal/webapi/handlers_channel_test.go:198 |
| `bad coeffs value: %s must be in [0,1]` | `collectCoeffUpdates` | engine/internal/webapi/handlers_coeffs.go:198 |
| `bad coeffs value: time.night_threshold_min must be >= 1` | `collectCoeffUpdates` | engine/internal/webapi/handlers_coeffs.go:244 |
| `world_meta.source_book_id: %w` | `readSourceBookID` | engine/internal/webapi/handlers_forge.go:151 |
| `книга-источник не привязана к п...` | `forgeSourceBook` | engine/internal/webapi/handlers_forge.go:184 |
| `файл книги: %w` | `forgeSourceBook` | engine/internal/webapi/handlers_forge.go:193 |
| `книга полки %q не найдена` | `shelfBookByID` | engine/internal/webapi/handlers_forge.go:228 |
| `нет модели` | `TestAtlasForgeNoModel` | engine/internal/webapi/handlers_forge_test.go:183 |
| `webapi: config.yaml без секции llm` | `writeLlamaServerPath` | engine/internal/webapi/handlers_llama_install.go:235 |
| `llama-server not found after install` | `handleLlamaInstall` | engine/internal/webapi/handlers_llama_install.go:547 |
| `install script timed out` | `runLlamaInstallScript` | engine/internal/webapi/handlers_llama_install.go:597 |
| `%w: %s` | `runLlamaInstallScript` | engine/internal/webapi/handlers_llama_install.go:600 |
| `%w: status %d` | `fetchGeminiModels` | engine/internal/webapi/handlers_provider_models.go:120 |
| `gemini list models: status %d` | `fetchGeminiModels` | engine/internal/webapi/handlers_provider_models.go:122 |
| `duplicate provider %q` | `validateRoute` | engine/internal/webapi/handlers_settings.go:528 |
| `unknown provider %q` | `validateRoute` | engine/internal/webapi/handlers_settings.go:534 |
| `local must be last` | `validateRoute` | engine/internal/webapi/handlers_settings.go:539 |
| `webapi: config key %s not found` | `updateConfigScalars` | engine/internal/webapi/handlers_settings.go:2271 |
| `webapi: имя модели: %w` | `modelSourceName` | engine/internal/webapi/model_files.go:93 |
| `webapi: имя модели: пустое имя в url %q` | `modelSourceName` | engine/internal/webapi/model_files.go:97 |
| `%w: %q (allowed: 127.0.0.1, localhost)` | `Listen` | engine/internal/webapi/server.go:57 |
| `webapi: server is not listening` | `(*Server).Serve` | engine/internal/webapi/server.go:121 |
| `webapi: response writer does not support hijacking` | `(*statusWriter).Hijack` | engine/internal/webapi/server.go:189 |
| `webapi: bad web.port %q` | `parseWeb` | engine/internal/webapi/server.go:252 |
| `webapi: loopback %s %s: status %d` | `(*TelegramBot).loopbackJSON` | engine/internal/webapi/telegram_commands.go:139 |
| `webapi: forward chat: status %d` | `(*TelegramBot).ForwardChat` | engine/internal/webapi/tunnel.go:424 |
| `webapi: telegram getUpdates: ok=false: %s` | `(*TelegramBot).GetUpdates` | engine/internal/webapi/tunnel.go:493 |
| `webapi: telegram answerCallbackQuery: status %d` | `(*TelegramBot).answerCallback` | engine/internal/webapi/tunnel.go:619 |
| `webapi: websocket frame too large` | `readServerFrame` | engine/internal/webapi/tunnel.go:888 |
| `webapi: telegram sendMessage: status %d` | `(*TelegramBot).sendTelegramMessage` | engine/internal/webapi/tunnel.go:937 |
| `webapi: chat state: status %d` | `(*TelegramBot).turnReply` | engine/internal/webapi/tunnel.go:964 |
| `webapi: неизвестный настраиваемый...` | `writeWorldTunables` | engine/internal/webapi/world_readonly.go:177 |
| `webapi: websocket frame too large` | `(*client).readFrame` | engine/internal/webapi/ws.go:257 |
| `webapi: unmasked client frame` | `(*client).readFrame` | engine/internal/webapi/ws.go:260 |
| `workshop: нет хэндла литературной б...` | `InsertBook` | engine/internal/workshop/book.go:36 |
| `workshop: книга без имени, языка или...` | `InsertBook` | engine/internal/workshop/book.go:39 |
| `workshop: нет хэндла мира для чтени�...` | `WorldRating` | engine/internal/workshop/censor_profile.go:39 |
| `workshop: разбор world_meta.%s: %w` | `WorldRating` | engine/internal/workshop/censor_profile.go:51 |
| `workshop: читать arcs.json %s: %w` | `LoadArcs` | engine/internal/workshop/complement.go:30 |
| `workshop: arcs.json %s: %w` | `LoadArcs` | engine/internal/workshop/complement.go:34 |
| `workshop: слот ковки карточки не по�...` | `RunForgeCard` | engine/internal/workshop/complement.go:213 |
| `workshop: карточка не JSON: %w` | `WriteComplementCard` | engine/internal/workshop/complement.go:252 |
| `workshop: карточка без психотипа ил...` | `WriteComplementCard` | engine/internal/workshop/complement.go:255 |
| `workshop: карточка не JSON: %w` | `validateCard` | engine/internal/workshop/complement.go:311 |
| `workshop: карточка без архетипа` | `validateCard` | engine/internal/workshop/complement.go:314 |
| `workshop: карточка без слоёв` | `validateCard` | engine/internal/workshop/complement.go:318 |
| `workshop: здоровье вне 0–100: %d` | `validateCard` | engine/internal/workshop/complement.go:321 |
| `workshop: навыков %d, want 9` | `validateCard` | engine/internal/workshop/complement.go:324 |
| `workshop: параметры памяти не запол...` | `validateCard` | engine/internal/workshop/complement.go:327 |
| `workshop: статов %d, want 6` | `validateCard` | engine/internal/workshop/complement.go:330 |
| `workshop: пороков %d, want 8` | `validateCard` | engine/internal/workshop/complement.go:333 |
| `workshop: гуморов %d, want 4` | `validateCard` | engine/internal/workshop/complement.go:336 |
| `workshop: скрытый слой без психотипа` | `validateCard` | engine/internal/workshop/complement.go:339 |
| `workshop: скрытый слой без мотивов` | `validateCard` | engine/internal/workshop/complement.go:342 |
| `workshop: ковка без origin` | `ForgeComposition` | engine/internal/workshop/compose.go:138 |
| `workshop: семья %q вне реестра` | `validateComposition` | engine/internal/workshop/compose.go:159 |
| `workshop: pattern_secondary %q совпадает с fami...` | `validateComposition` | engine/internal/workshop/compose.go:163 |
| `workshop: pattern_secondary %q вне реестра` | `validateComposition` | engine/internal/workshop/compose.go:166 |
| `workshop: техника %q вне списка` | `validateComposition` | engine/internal/workshop/compose.go:170 |
| `workshop: моральная позиция %q вне с�...` | `validateComposition` | engine/internal/workshop/compose.go:173 |
| `workshop: нет хэндла мира для редак�...` | `EditChapter` | engine/internal/workshop/editor.go:66 |
| `workshop: слот Редактора не подключ...` | `EditChapter` | engine/internal/workshop/editor.go:69 |
| `workshop: нет собранной версии глав...` | `readEditorSource` | engine/internal/workshop/editor.go:114 |
| `workshop: Редактор не вернул правки` | `askEditorEdits` | engine/internal/workshop/editor.go:190 |
| `workshop: лишние данные после JSON-пр�...` | `parseEditorEdits` | engine/internal/workshop/editor.go:206 |
| `workshop: пустая сторона правки` | `parseEditorEdits` | engine/internal/workshop/editor.go:210 |
| `workshop: правка «было == стало»` | `parseEditorEdits` | engine/internal/workshop/editor.go:213 |
| `workshop: Редактор не вернул кратко...` | `askEditorSummary` | engine/internal/workshop/editor.go:299 |
| `workshop: лишние данные после JSON-со�...` | `parseEditorSummary` | engine/internal/workshop/editor.go:314 |
| `workshop: пустое краткое содержание` | `parseEditorSummary` | engine/internal/workshop/editor.go:317 |
| `workshop: нет хэндла мира для очере�...` | `EnqueueEditorTask` | engine/internal/workshop/editor_queue.go:33 |
| `workshop: id главы для редактуры не з...` | `EnqueueEditorTask` | engine/internal/workshop/editor_queue.go:36 |
| `workshop: нет хэндла мира для очере�...` | `DequeueEditorTask` | engine/internal/workshop/editor_queue.go:56 |
| `workshop: payload заказа edit: %w` | `DequeueEditorTask` | engine/internal/workshop/editor_queue.go:72 |
| `workshop: заказ edit без id главы` | `DequeueEditorTask` | engine/internal/workshop/editor_queue.go:75 |
| `workshop: нет задачи редактуры для �...` | `RunEditorTask` | engine/internal/workshop/editor_queue.go:91 |
| `слот: тайм-аут` | `TestEditorJournalRefuse` | engine/internal/workshop/editor_test.go:294 |
| `workshop: эпоха не выведена из опис�...` | `ForgeEpoch` | engine/internal/workshop/epoch.go:124 |
| `workshop: слот ковки справочника эп...` | `ForgeEpoch` | engine/internal/workshop/epoch.go:145 |
| `workshop: авто-критик не провёл рев�...` | `ForgeEpoch` | engine/internal/workshop/epoch.go:159 |
| `%w: %s` | `ForgeEpoch` | engine/internal/workshop/epoch.go:164 |
| `workshop: нет хэндла мира для чтени�...` | `readWorldMetaString` | engine/internal/workshop/epoch.go:252 |
| `workshop: world_meta.%s не найден` | `readWorldMetaString` | engine/internal/workshop/epoch.go:257 |
| `workshop: разбор world_meta.%s: %w` | `readWorldMetaString` | engine/internal/workshop/epoch.go:264 |
| `workshop: ковка чанка %d: %w` | `RunForgeBook` | engine/internal/workshop/forge_book.go:101 |
| `workshop: разбор книги %d: %w` | `RunForgeBook` | engine/internal/workshop/forge_book.go:121 |
| `workshop: вектор чанка книги: %w` | `writeBankVector` | engine/internal/workshop/forge_book.go:181 |
| `workshop: вектор чанка книги dim=%d, want %d` | `writeBankVector` | engine/internal/workshop/forge_book.go:184 |
| `workshop: читать вход ковки %s: %w` | `ParseBookInput` | engine/internal/workshop/forge_input.go:73 |
| `%w: %s (принимаются только txt и fb2)` | `unsupportedFormat` | engine/internal/workshop/forge_input.go:173 |
| `workshop: читать вход ковки %s: %w` | `NormalizeBookFile` | engine/internal/workshop/forge_input.go:287 |
| `workshop: писать utf-8 вход ковки %s: %w` | `writeBookUTF8` | engine/internal/workshop/forge_input.go:307 |
| `workshop: заменить вход ковки %s: %w` | `writeBookUTF8` | engine/internal/workshop/forge_input.go:311 |
| `workshop: вход ковки: кодировка %q: %w` | `toUTF8` | engine/internal/workshop/forge_input.go:347 |
| `workshop: вход ковки: кодировка %q не...` | `toUTF8` | engine/internal/workshop/forge_input.go:353 |
| `workshop: вход ковки: кодировка %q: %w` | `toUTF8` | engine/internal/workshop/forge_input.go:357 |
| `%w: кандидатов нет` | `heuristicDecode` | engine/internal/workshop/forge_input.go:391 |
| `%w: %s` | `heuristicDecode` | engine/internal/workshop/forge_input.go:399 |
| `workshop: декодировать вход в utf-8: %w` | `decodeWith` | engine/internal/workshop/forge_input.go:557 |
| `workshop: fb2: кодировка %q не поддерж�...` | `charsetReader` | engine/internal/workshop/forge_input.go:622 |
| `workshop: fb2: %w` | `parseFB2` | engine/internal/workshop/forge_input.go:684 |
| `workshop: делексификация: %w` | `Delexify` | engine/internal/workshop/forge_pipeline.go:59 |
| `workshop: ковка без origin` | `ForgeBankRecord` | engine/internal/workshop/forge_pipeline.go:70 |
| `workshop: sediment_scene %d не найдена` | `RemeltScene` | engine/internal/workshop/forge_pipeline.go:137 |
| `%w: trust_level=%d` | `RemeltScene` | engine/internal/workshop/forge_pipeline.go:143 |
| `workshop: слот генерации не подключ...` | `ladder` | engine/internal/workshop/generate.go:95 |
| `workshop: слот ковки гороскопа не п�...` | `RunHoroscopeForge` | engine/internal/workshop/horoscope.go:163 |
| `workshop: выход ковки гороскопа не J...` | `RunHoroscopeForge` | engine/internal/workshop/horoscope.go:176 |
| `workshop: Цензор не пропустил ветку...` | `RunHoroscopeForge` | engine/internal/workshop/horoscope.go:188 |
| `workshop: эмбеддер не подключён` | `RunHoroscopeForge` | engine/internal/workshop/horoscope.go:195 |
| `workshop: эмбеддинг ветки гороскоп�...` | `RunHoroscopeForge` | engine/internal/workshop/horoscope.go:205 |
| `workshop: вектор ветки dim=%d, want %d` | `RunHoroscopeForge` | engine/internal/workshop/horoscope.go:208 |
| `workshop: прогноз Пилы вне echo|contrapunt: %q` | `validateHoroscopeOutput` | engine/internal/workshop/horoscope.go:261 |
| `workshop: гороскоп без веток` | `validateHoroscopeOutput` | engine/internal/workshop/horoscope.go:264 |
| `workshop: id ветки вне neg|pos|aside: %q` | `validateHoroscopeOutput` | engine/internal/workshop/horoscope.go:268 |
| `workshop: kind ветки вне bank_ref|weaver_prompt: %q` | `validateHoroscopeOutput` | engine/internal/workshop/horoscope.go:271 |
| `workshop: ветка без probe_text` | `validateHoroscopeOutput` | engine/internal/workshop/horoscope.go:274 |
| `workshop: матчер гороскопа без хэнд...` | `MatchHoroscope` | engine/internal/workshop/horoscope_match.go:97 |
| `workshop: issue_json гороскопа не JSON: %w` | `MatchHoroscope` | engine/internal/workshop/horoscope_match.go:121 |
| `workshop: разбор globalLuck: %w` | `(*Luck).Get` | engine/internal/workshop/luck.go:76 |
| `workshop: globalLuck вне диапазона [0,1]: %v` | `(*Luck).Set` | engine/internal/workshop/luck.go:85 |
| `workshop: нет хэндла для записи globalLuck` | `(*Luck).Set` | engine/internal/workshop/luck.go:88 |
| `workshop: нет хэндла для записи сна�...` | `(*Luck).SaveCheckpoint` | engine/internal/workshop/luck.go:150 |
| `workshop: сериализация снапшота ГС�...` | `(*Luck).SaveCheckpoint` | engine/internal/workshop/luck.go:154 |
| `workshop: разбор снапшота ГСЧ: %w` | `(*Luck).LoadCheckpoint` | engine/internal/workshop/luck.go:178 |
| `workshop: нет хэндла журнала событий` | `RecordRoll` | engine/internal/workshop/luck.go:195 |
| `workshop: сериализация броска: %w` | `RecordRoll` | engine/internal/workshop/luck.go:199 |
| `workshop: разбор события броска: %w` | `LoadRollEvents` | engine/internal/workshop/luck.go:223 |
| `workshop: слот Пифии не подключён` | `RunPythia` | engine/internal/workshop/pythia.go:84 |
| `workshop: выход Пифии не JSON: %w` | `RunPythia` | engine/internal/workshop/pythia.go:97 |
| `workshop: world_meta.hero_birth не найден` | `heroBirth` | engine/internal/workshop/pythia.go:121 |
| `workshop: разбор hero_birth: %w` | `heroBirth` | engine/internal/workshop/pythia.go:128 |
| `workshop: карточка героя (characters id 0) �...` | `playerPsychotype` | engine/internal/workshop/pythia.go:141 |
| `workshop: выход Пифии без настроения` | `validatePythiaOutput` | engine/internal/workshop/pythia.go:193 |
| `workshop: выход Пифии без граней ка�...` | `validatePythiaOutput` | engine/internal/workshop/pythia.go:196 |
| `workshop: payload заказа %s: %w` | `Enqueue` | engine/internal/workshop/queue.go:197 |
| `workshop: разбор favorite_books: %w` | `favoriteBooks` | engine/internal/workshop/queue.go:289 |
| `workshop: нет хэндла мира для оседа�...` | `RunSedimentOrder` | engine/internal/workshop/sediment_run.go:68 |
| `workshop: нет заказа оседания` | `RunSedimentOrder` | engine/internal/workshop/sediment_run.go:71 |
| `workshop: payload заказа sediment: %w` | `loadSceneForSediment` | engine/internal/workshop/sediment_run.go:130 |
| `workshop: заказ sediment без turn_id` | `loadSceneForSediment` | engine/internal/workshop/sediment_run.go:133 |
| `workshop: сцена заказа sediment (turn_id=%d) �...` | `loadSceneForSediment` | engine/internal/workshop/sediment_run.go:141 |
| `workshop: нет хэндла мира для сбора ...` | `CollectSamples` | engine/internal/workshop/style.go:115 |
| `workshop: стилизация без черновика` | `StylizeClassic` | engine/internal/workshop/style.go:263 |
| `workshop: слот стилизации не подклю...` | `StylizeClassic` | engine/internal/workshop/style.go:266 |
| `workshop: пустой выход стилизации` | `StylizeClassic` | engine/internal/workshop/style.go:277 |

