# Concurrency Primitives

Total: 439 sites across 9 kind(s).

> Review these sites before any concurrent refactor.

## channel_send (65)

| Function | File | Detail |
|----------|------|--------|
| `main` | Levant/generate_SVG_map.go:95 | sem <- |
| `runTunnelScript` | engine/cmd/tns65/main.go:571 | done <- |
| `(*liveScoutSlot).Stream` | engine/internal/birth/deep_scout_test.go:45 | ch <- |
| `TestWake` | engine/internal/book/wake_test.go:124 | textCh <- |
| `TestWake` | engine/internal/book/wake_test.go:125 | errCh <- |
| `TestTurnPriority` | engine/internal/core/sched_test.go:153 | guardCh <- |
| `checkHealth` | engine/internal/embedserver/embedserver.go:131 | errCh <- |
| `(*Client).pump` | engine/internal/lemmarpc/client.go:198 | ch <- |
| `(*Client).pump` | engine/internal/lemmarpc/client.go:211 | ch <- |
| `startEmbedServer` | engine/internal/models/embed_test.go:34 | reqs <- |
| `DownloadFile` | engine/internal/models/fetch.go:186 | errCh <- |
| `(*OpenAIClient).Stream` | engine/internal/models/openai.go:156 | ch <- |
| `(*ProviderClient).Stream` | engine/internal/models/provider.go:311 | ch <- |
| `(*fakeChat).Stream` | engine/internal/models/provider_test.go:33 | ch <- |
| `(*Weaver).Stream` | engine/internal/models/weaver.go:100 | ch <- |
| `chunkServer` | engine/internal/models/weaver_test.go:26 | prompts <- |
| `(*liveSlot).Stream` | engine/internal/paganel/deep_scout_test.go:48 | ch <- |
| `(*Scout).Fetch` | engine/internal/paganel/scout.go:123 | done <- |
| `(*Scout).Fetch` | engine/internal/paganel/scout.go:129 | done <- |
| `(*Scout).Fetch` | engine/internal/paganel/scout.go:133 | done <- |
| `(*Scout).Fetch` | engine/internal/paganel/scout.go:136 | done <- |
| `TestScout` | engine/internal/paganel/scout_test.go:97 | done <- |
| `(*fakeClient).Stream` | engine/internal/providerroute/router_test.go:56 | ch <- |
| `(*child).run` | engine/internal/supervisor/child.go:124 | waitCh <- |
| `(*fakeClock).After` | engine/internal/supervisor/supervisor_test.go:42 | ch <- |
| `TestRestart` | engine/internal/supervisor/supervisor_test.go:184 | crashCh <- |
| `(*Clock).Advance` | engine/internal/testkit/clock.go:48 | t.ch <- |
| `TestFakeSlot` | engine/internal/testkit/testkit_test.go:80 | done <- |
| `TestFeedbackEscalation` | engine/internal/turn/feedback_test.go:88 | ch <- |
| `TestFeedbackEscalation` | engine/internal/turn/feedback_test.go:115 | ch2 <- |
| `sceneChunk` | engine/internal/turn/idiolect_test.go:16 | ch <- |
| `(*notifiedClock).After` | engine/internal/turn/safety_test.go:55 | c.after <- |
| `TestSilenceThreshold` | engine/internal/turn/safety_test.go:73 | ch <- |
| `TestSilenceThreshold` | engine/internal/turn/safety_test.go:79 | done <- |
| `TestSilenceThreshold` | engine/internal/turn/safety_test.go:104 | done <- |
| `TestSilenceThreshold` | engine/internal/turn/safety_test.go:133 | done <- |
| `TestSilenceThreshold` | engine/internal/turn/safety_test.go:169 | ch <- |
| `TestSilenceThreshold` | engine/internal/turn/safety_test.go:170 | ch <- |
| `streamScene` | engine/internal/turn/stream.go:70 | ch <- |
| `streamScene` | engine/internal/turn/stream.go:77 | ch <- |
| `TestStream` | engine/internal/turn/stream_test.go:62 | slot.ch <- |
| `TestStream` | engine/internal/turn/stream_test.go:88 | ch <- |
| `TestStream` | engine/internal/turn/stream_test.go:89 | ch <- |
| `TestStream` | engine/internal/turn/stream_test.go:125 | ch <- |
| `TestWeaverCallJournal` | engine/internal/turn/stream_test.go:211 | ch <- |
| `restartTelegramChannel` | engine/internal/webapi/handlers_settings.go:921 | done <- |
| `TestTelegramBotForeignChatIgnored` | engine/internal/webapi/telegram_commands_test.go:83 | loopCh <- |
| `TestTelegramBotForeignChatIgnored` | engine/internal/webapi/telegram_commands_test.go:92 | tgCh <- |
| `TestStartTelegramLifecycle` | engine/internal/webapi/telegram_runtime_test.go:47 | tgCh <- |
| `TestTelegramBotForwardsChat` | engine/internal/webapi/tunnel_test.go:149 | loopCh <- |
| `TestTelegramBotForwardsChat` | engine/internal/webapi/tunnel_test.go:157 | tgCh <- |
| `TestTelegramBotLongPoll` | engine/internal/webapi/tunnel_test.go:232 | loopCh <- |
| `TestTelegramBotNoActiveWorld` | engine/internal/webapi/tunnel_test.go:288 | tgCh <- |
| `TestTelegramBotNoActiveWorld` | engine/internal/webapi/tunnel_test.go:304 | loopCalled <- |
| `TestTelegramBotSubscribeNews` | engine/internal/webapi/tunnel_test.go:404 | tgCh <- |
| `TestTelegramBotSubscribeNews` | engine/internal/webapi/tunnel_test.go:427 | done <- |
| `TestTelegramBotRoundTrip` | engine/internal/webapi/tunnel_test.go:490 | loopCh <- |
| `TestTelegramBotRoundTrip` | engine/internal/webapi/tunnel_test.go:512 | tgCh <- |
| `TestTelegramBotLoopbackErrorDoesNotStopPoll` | engine/internal/webapi/tunnel_test.go:584 | tgCh <- |
| `TestTelegramBotRelaysTooLongHint` | engine/internal/webapi/tunnel_test.go:644 | tgCh <- |
| `TestTelegramBotStaleTurnReplyNotSent` | engine/internal/webapi/tunnel_test.go:707 | tgCh <- |
| `newCallbackBot` | engine/internal/webapi/tunnel_test.go:783 | loopCh <- |
| `newCallbackBot` | engine/internal/webapi/tunnel_test.go:794 | tgCh <- |
| `TestTelegramBotSignalErrorAnswer` | engine/internal/webapi/tunnel_test.go:981 | tgCh <- |
| `(*fakeRouteSlot).Stream` | engine/internal/workshop/routed_slot_test.go:36 | ch <- |

## goroutine (44)

| Function | File | Detail |
|----------|------|--------|
| `main` | Levant/generate_SVG_map.go:93 | go  |
| `run` | engine/cmd/tns65/main.go:196 | go  |
| `runTunnelScript` | engine/cmd/tns65/main.go:570 | go  |
| `Birth` | engine/internal/birth/birth.go:227 | go  |
| `TestWake` | engine/internal/book/wake_test.go:122 | go  |
| `TestTurnPriority` | engine/internal/core/sched_test.go:37 | go  |
| `TestTurnPriority` | engine/internal/core/sched_test.go:90 | go  |
| `TestTurnPriority` | engine/internal/core/sched_test.go:153 | go  |
| `TestTurnPriority` | engine/internal/core/sched_test.go:164 | go  |
| `checkHealth` | engine/internal/embedserver/embedserver.go:131 | go  |
| `TestRotation` | engine/internal/journal/journal_test.go:153 | go  |
| `(*Client).startLocked` | engine/internal/lemmarpc/client.go:154 | go c.pump |
| `DownloadFile` | engine/internal/models/fetch.go:183 | go  |
| `(*OpenAIClient).Stream` | engine/internal/models/openai.go:138 | go  |
| `(*ProviderClient).Stream` | engine/internal/models/provider.go:297 | go  |
| `(*fakeChat).Stream` | engine/internal/models/provider_test.go:29 | go  |
| `(*Weaver).Stream` | engine/internal/models/weaver.go:90 | go  |
| `(*Scout).Fetch` | engine/internal/paganel/scout.go:120 | go  |
| `TestScout` | engine/internal/paganel/scout_test.go:95 | go  |
| `(*child).run` | engine/internal/supervisor/child.go:124 | go  |
| `TestSecondInstance` | engine/internal/supervisor/lock_test.go:59 | go s.Run |
| `(*Supervisor).SwitchModel` | engine/internal/supervisor/status.go:68 | go  |
| `(*Supervisor).SwitchModel` | engine/internal/supervisor/status.go:78 | go  |
| `(*Supervisor).Run` | engine/internal/supervisor/supervisor.go:136 | go  |
| `TestRestart` | engine/internal/supervisor/supervisor_test.go:179 | go s.Run |
| `TestNoShortReadyBudget` | engine/internal/supervisor/supervisor_test.go:243 | go s.Run |
| `TestStopOrder` | engine/internal/supervisor/supervisor_test.go:279 | go s.Run |
| `TestSwitchModel` | engine/internal/supervisor/supervisor_test.go:396 | go s.Run |
| `TestSwitchModelRollback` | engine/internal/supervisor/supervisor_test.go:435 | go s.Run |
| `TestStopChild` | engine/internal/supervisor/supervisor_test.go:466 | go s.Run |
| `TestChildLogPath` | engine/internal/supervisor/supervisor_test.go:505 | go s.Run |
| `TestStatus` | engine/internal/supervisor/supervisor_test.go:541 | go s.Run |
| `TestFakeSlot` | engine/internal/testkit/testkit_test.go:78 | go  |
| `TestSilenceThreshold` | engine/internal/turn/safety_test.go:77 | go  |
| `TestSilenceThreshold` | engine/internal/turn/safety_test.go:102 | go  |
| `TestSilenceThreshold` | engine/internal/turn/safety_test.go:132 | go  |
| `streamScene` | engine/internal/turn/stream.go:66 | go  |
| `restartTelegramChannel` | engine/internal/webapi/handlers_settings.go:918 | go  |
| `StartTelegram` | engine/internal/webapi/tunnel.go:345 | go  |
| `StartTelegram` | engine/internal/webapi/tunnel.go:348 | go  |
| `StartTelegram` | engine/internal/webapi/tunnel.go:360 | go  |
| `(*TelegramBot).SubscribeNews` | engine/internal/webapi/tunnel.go:698 | go  |
| `TestTelegramBotSubscribeNews` | engine/internal/webapi/tunnel_test.go:426 | go  |
| `(*Hub).serveHTTP` | engine/internal/webapi/ws.go:121 | go c.readLoop |

## mutex_lock (132)

| Function | File | Detail |
|----------|------|--------|
| `main` | Levant/generate_SVG_map.go:99 | mu.Lock |
| `startTelegramBot` | engine/cmd/tns65/main.go:62 | botMu.Lock |
| `run` | engine/cmd/tns65/main.go:209 | botMu.Lock |
| `(*childRunner).switchModel` | engine/cmd/tns65/main.go:287 | r.mu.Lock |
| `(*childRunner).stopChildByName` | engine/cmd/tns65/main.go:307 | r.mu.Lock |
| `(*childRunner).status` | engine/cmd/tns65/main.go:323 | r.mu.Lock |
| `(*childRunner).stop` | engine/cmd/tns65/main.go:333 | r.mu.Lock |
| `(*PatternCache).Lookup` | engine/internal/bank/bank.go:124 | c.mu.Lock |
| `(*PatternCache).Remember` | engine/internal/bank/bank.go:136 | c.mu.Lock |
| `(*PatternCache).Known` | engine/internal/bank/bank.go:147 | c.mu.Lock |
| `(*PatternCache).Hits` | engine/internal/bank/bank.go:155 | c.mu.Lock |
| `(*PatternCache).Len` | engine/internal/bank/bank.go:165 | c.mu.Lock |
| `(*countingEmbedder).Embed` | engine/internal/birth/anchorbench_test.go:61 | e.mu.Lock |
| `(*countingEmbedder).Calls` | engine/internal/birth/anchorbench_test.go:68 | e.mu.Lock |
| `Close` | engine/internal/censor/censor.go:106 | checkersMu.Lock |
| `checkerFor` | engine/internal/censor/censor.go:124 | checkersMu.Lock |
| `(*Scheduler).AcquireWorkshop` | engine/internal/core/sched.go:38 | s.mu.Lock |
| `(*Scheduler).AcquireWorkshop` | engine/internal/core/sched.go:50 | s.mu.Lock |
| `(*Scheduler).BeginTurn` | engine/internal/core/sched.go:61 | s.mu.Lock |
| `(*TurnGuard).End` | engine/internal/core/sched.go:76 | g.s.mu.Lock |
| `(*Scheduler).WorkshopBusy` | engine/internal/core/sched.go:87 | s.mu.Lock |
| `(*Scheduler).WorkshopWaiting` | engine/internal/core/sched.go:94 | s.mu.Lock |
| `(*Run).Lemmatize` | engine/internal/deepread/lemmas.go:68 | r.mu.Lock |
| `(*Run).Err` | engine/internal/deepread/lemmas.go:98 | r.mu.Lock |
| `(*Run).Close` | engine/internal/deepread/lemmas.go:105 | r.mu.Lock |
| `(*Logger).Event` | engine/internal/journal/journal.go:62 | l.mu.Lock |
| `(*Logger).OnEvent` | engine/internal/journal/journal.go:102 | l.mu.Lock |
| `(*Client).Lemmatize` | engine/internal/lemmarpc/client.go:69 | c.mu.Lock |
| `(*Client).Lemmatize` | engine/internal/lemmarpc/client.go:99 | c.mu.Lock |
| `(*Client).Lemmatize` | engine/internal/lemmarpc/client.go:109 | c.mu.Lock |
| `(*Client).Close` | engine/internal/lemmarpc/client.go:125 | c.mu.Lock |
| `(*Client).handleReadError` | engine/internal/lemmarpc/client.go:172 | c.mu.Lock |
| `(*Client).pump` | engine/internal/lemmarpc/client.go:195 | c.mu.Lock |
| `(*Client).pump` | engine/internal/lemmarpc/client.go:204 | c.mu.Lock |
| `(*progressTracker).add` | engine/internal/models/fetch.go:489 | p.mu.Lock |
| `(*fetchServer).ServeHTTP` | engine/internal/models/fetch_test.go:49 | s.mu.Lock |
| `(*fetchServer).rangeHeaders` | engine/internal/models/fetch_test.go:80 | s.mu.Lock |
| `(*OpenAIClient).LimitRemaining` | engine/internal/models/openai.go:61 | c.mu.Lock |
| `(*OpenAIClient).setLimit` | engine/internal/models/openai.go:67 | c.mu.Lock |
| `(*ProviderClient).LimitRemaining` | engine/internal/models/provider.go:215 | c.mu.Lock |
| `(*ProviderClient).setLimit` | engine/internal/models/provider.go:221 | c.mu.Lock |
| `(*Budget).ScoutSpend` | engine/internal/paganel/corridor.go:224 | b.mu.Lock |
| `(*Budget).BuildSpend` | engine/internal/paganel/corridor.go:236 | b.mu.Lock |
| `(*Budget).ScoutSpent` | engine/internal/paganel/corridor.go:247 | b.mu.Lock |
| `(*Budget).BuildSpent` | engine/internal/paganel/corridor.go:254 | b.mu.Lock |
| `(*DeepScout).Spent` | engine/internal/paganel/deep_scout.go:84 | s.mu.Lock |
| `(*DeepScout).takeBudget` | engine/internal/paganel/deep_scout.go:121 | s.mu.Lock |
| `(*Router).candidates` | engine/internal/providerroute/router.go:141 | r.mu.Lock |
| `(*Router).delivered` | engine/internal/providerroute/router.go:178 | r.mu.Lock |
| `(*Router).markFailure` | engine/internal/providerroute/router.go:187 | r.mu.Lock |
| `(*Router).client` | engine/internal/providerroute/router.go:203 | r.mu.Lock |
| `(*fakeClient).Stream` | engine/internal/providerroute/router_test.go:47 | f.mu.Lock |
| `(*fakeClient).callCount` | engine/internal/providerroute/router_test.go:66 | f.mu.Lock |
| `(*RoutedProseSlot).ProviderName` | engine/internal/providerroute/slot.go:77 | s.mu.Lock |
| `(*RoutedProseSlot).LimitRemaining` | engine/internal/providerroute/slot.go:85 | s.mu.Lock |
| `(*RoutedProseSlot).setMeta` | engine/internal/providerroute/slot.go:91 | s.mu.Lock |
| `(*child).terminate` | engine/internal/supervisor/child.go:220 | c.mu.Lock |
| `(*child).setProc` | engine/internal/supervisor/child.go:244 | c.mu.Lock |
| `(*child).snapshot` | engine/internal/supervisor/child.go:253 | c.mu.Lock |
| `(*Supervisor).Run` | engine/internal/supervisor/supervisor.go:123 | s.mu.Lock |
| `(*Supervisor).Stop` | engine/internal/supervisor/supervisor.go:146 | s.mu.Lock |
| `(*fakeClock).Now` | engine/internal/supervisor/supervisor_test.go:31 | c.mu.Lock |
| `(*fakeClock).After` | engine/internal/supervisor/supervisor_test.go:37 | c.mu.Lock |
| `(*fakeRunner).Start` | engine/internal/supervisor/supervisor_test.go:96 | r.mu.Lock |
| `(*fakeRunner).lastArgs` | engine/internal/supervisor/supervisor_test.go:107 | r.mu.Lock |
| `(*fakeRunner).recordStop` | engine/internal/supervisor/supervisor_test.go:116 | r.mu.Lock |
| `(*fakeRunner).startCount` | engine/internal/supervisor/supervisor_test.go:122 | r.mu.Lock |
| `(*fakeRunner).stopOrder` | engine/internal/supervisor/supervisor_test.go:128 | r.mu.Lock |
| `TestRestart` | engine/internal/supervisor/supervisor_test.go:167 | mu.Lock |
| `TestRestart` | engine/internal/supervisor/supervisor_test.go:183 | mu.Lock |
| `TestNoShortReadyBudget` | engine/internal/supervisor/supervisor_test.go:216 | mu.Lock |
| `TestNoShortReadyBudget` | engine/internal/supervisor/supervisor_test.go:234 | mu.Lock |
| `TestNoShortReadyBudget` | engine/internal/supervisor/supervisor_test.go:247 | mu.Lock |
| `TestNoShortReadyBudget` | engine/internal/supervisor/supervisor_test.go:252 | mu.Lock |
| `TestChildLogPath` | engine/internal/supervisor/supervisor_test.go:508 | runner.mu.Lock |
| `(*Clock).Now` | engine/internal/testkit/clock.go:31 | c.mu.Lock |
| `(*Clock).Advance` | engine/internal/testkit/clock.go:41 | c.mu.Lock |
| `(*Clock).After` | engine/internal/testkit/clock.go:57 | c.mu.Lock |
| `(*Clock).PendingTimers` | engine/internal/testkit/clock.go:68 | c.mu.Lock |
| `(*FakeEmbed).Embed` | engine/internal/testkit/fakeembed.go:27 | e.mu.Lock |
| `(*FakeEmbed).Calls` | engine/internal/testkit/fakeembed.go:35 | e.mu.Lock |
| `(*FakeSlot).SetResponse` | engine/internal/testkit/fakeslot.go:33 | s.mu.Lock |
| `(*FakeSlot).SetDelay` | engine/internal/testkit/fakeslot.go:41 | s.mu.Lock |
| `(*FakeSlot).SetClock` | engine/internal/testkit/fakeslot.go:48 | s.mu.Lock |
| `(*FakeSlot).FailWith` | engine/internal/testkit/fakeslot.go:55 | s.mu.Lock |
| `(*FakeSlot).Calls` | engine/internal/testkit/fakeslot.go:62 | s.mu.Lock |
| `(*FakeSlot).SetCallLimit` | engine/internal/testkit/fakeslot.go:70 | s.mu.Lock |
| `(*FakeSlot).Generate` | engine/internal/testkit/fakeslot.go:78 | s.mu.Lock |
| `(*SlotSpy).Generate` | engine/internal/testkit/slotspy.go:33 | s.mu.Lock |
| `(*SlotSpy).Generate` | engine/internal/testkit/slotspy.go:39 | s.mu.Lock |
| `(*SlotSpy).Calls` | engine/internal/testkit/slotspy.go:48 | s.mu.Lock |
| `(*SlotSpy).Timeouts` | engine/internal/testkit/slotspy.go:55 | s.mu.Lock |
| `(*runawayClock).Now` | engine/internal/turn/composition_test.go:283 | c.mu.Lock |
| `(*fakeProseSlot).Stream` | engine/internal/turn/safety_test.go:25 | s.mu.Lock |
| `(*fakeProseSlot).lastPrompt` | engine/internal/turn/safety_test.go:35 | s.mu.Lock |
| `setActiveSlug` | engine/internal/webapi/handlers_atlas.go:71 | activeMu.Lock |
| `getActiveSlug` | engine/internal/webapi/handlers_atlas.go:159 | activeMu.Lock |
| `chatReturnSummary` | engine/internal/webapi/handlers_chat.go:355 | chatLastActiveMu.Lock |
| `chatLastActiveAt` | engine/internal/webapi/handlers_chat.go:395 | chatLastActiveMu.Lock |
| `chatTouch` | engine/internal/webapi/handlers_chat.go:403 | chatLastActiveMu.Lock |
| `currentLlamaInstall` | engine/internal/webapi/handlers_llama_install.go:93 | llamaInstallMu.Lock |
| `beginLlamaInstall` | engine/internal/webapi/handlers_llama_install.go:100 | llamaInstallMu.Lock |
| `setLlamaInstallStage` | engine/internal/webapi/handlers_llama_install.go:114 | llamaInstallMu.Lock |
| `finishLlamaInstall` | engine/internal/webapi/handlers_llama_install.go:122 | llamaInstallMu.Lock |
| `setChannelRestartWarning` | engine/internal/webapi/handlers_settings.go:219 | channelRestartMu.Lock |
| `getChannelRestartWarning` | engine/internal/webapi/handlers_settings.go:225 | channelRestartMu.Lock |
| `setModelJob` | engine/internal/webapi/handlers_settings.go:1360 | modelJobsMu.Lock |
| `modelJob` | engine/internal/webapi/handlers_settings.go:1366 | modelJobsMu.Lock |
| `beginModelJob` | engine/internal/webapi/handlers_settings.go:1390 | modelJobsMu.Lock |
| `handleSettingsModelStatus` | engine/internal/webapi/handlers_settings.go:1661 | modelJobsMu.Lock |
| `newChildEmbedder` | engine/internal/webapi/handlers_settings_test.go:1279 | fe.mu.Lock |
| `newCaptureServer` | engine/internal/webapi/telegram_commands_test.go:28 | cs.mu.Lock |
| `(*captureServer).requests` | engine/internal/webapi/telegram_commands_test.go:41 | cs.mu.Lock |
| `(*Hub).Close` | engine/internal/webapi/ws.go:71 | h.mu.Lock |
| `(*Hub).Broadcast` | engine/internal/webapi/ws.go:84 | h.mu.Lock |
| `(*Hub).add` | engine/internal/webapi/ws.go:96 | h.mu.Lock |
| `(*Hub).remove` | engine/internal/webapi/ws.go:102 | h.mu.Lock |
| `(*client).setTopics` | engine/internal/webapi/ws.go:207 | c.mu.Lock |
| `(*client).send` | engine/internal/webapi/ws.go:217 | c.mu.Lock |
| `(*client).writeFrame` | engine/internal/webapi/ws.go:278 | c.mu.Lock |
| `(*bookMapProvider).requestBody` | engine/internal/workshop/bookmap_route_test.go:37 | p.mu.Lock |
| `newBookMapProvider` | engine/internal/workshop/bookmap_route_test.go:50 | p.mu.Lock |
| `(*roleCountingSlot).Generate` | engine/internal/workshop/editor_queue_test.go:24 | s.mu.Lock |
| `(*roleCountingSlot).roleCalls` | engine/internal/workshop/editor_queue_test.go:31 | s.mu.Lock |
| `(*PauseTimer).MarkActive` | engine/internal/workshop/pause.go:83 | t.mu.Lock |
| `(*PauseTimer).Silence` | engine/internal/workshop/pause.go:90 | t.mu.Lock |
| `(*recordSlot).Generate` | engine/internal/workshop/pause_test.go:78 | s.mu.Lock |
| `(*recordSlot).Generate` | engine/internal/workshop/pause_test.go:90 | s.mu.Lock |
| `(*Scheduler).SetSedimentRunner` | engine/internal/workshop/scheduler.go:64 | s.mu.Lock |
| `(*Scheduler).SetChapterClosed` | engine/internal/workshop/scheduler.go:77 | s.mu.Lock |
| `(*Scheduler).Step` | engine/internal/workshop/scheduler.go:84 | s.mu.Lock |
| `(*Scheduler).runOne` | engine/internal/workshop/scheduler.go:177 | s.mu.Lock |

## mutex_unlock (141)

| Function | File | Detail |
|----------|------|--------|
| `main` | Levant/generate_SVG_map.go:101 | mu.Unlock |
| `startTelegramBot` | engine/cmd/tns65/main.go:67 | botMu.Unlock |
| `run` | engine/cmd/tns65/main.go:210 | botMu.Unlock |
| `(*childRunner).switchModel` | engine/cmd/tns65/main.go:288 | r.mu.Unlock |
| `(*childRunner).stopChildByName` | engine/cmd/tns65/main.go:311 | r.mu.Unlock |
| `(*childRunner).stopChildByName` | engine/cmd/tns65/main.go:316 | r.mu.Unlock |
| `(*childRunner).status` | engine/cmd/tns65/main.go:324 | r.mu.Unlock |
| `(*childRunner).stop` | engine/cmd/tns65/main.go:335 | r.mu.Unlock |
| `(*PatternCache).Lookup` | engine/internal/bank/bank.go:125 | c.mu.Unlock |
| `(*PatternCache).Remember` | engine/internal/bank/bank.go:137 | c.mu.Unlock |
| `(*PatternCache).Known` | engine/internal/bank/bank.go:148 | c.mu.Unlock |
| `(*PatternCache).Hits` | engine/internal/bank/bank.go:156 | c.mu.Unlock |
| `(*PatternCache).Len` | engine/internal/bank/bank.go:166 | c.mu.Unlock |
| `(*countingEmbedder).Embed` | engine/internal/birth/anchorbench_test.go:63 | e.mu.Unlock |
| `(*countingEmbedder).Calls` | engine/internal/birth/anchorbench_test.go:69 | e.mu.Unlock |
| `Close` | engine/internal/censor/censor.go:107 | checkersMu.Unlock |
| `checkerFor` | engine/internal/censor/censor.go:125 | checkersMu.Unlock |
| `(*Scheduler).AcquireWorkshop` | engine/internal/core/sched.go:45 | s.mu.Unlock |
| `(*Scheduler).AcquireWorkshop` | engine/internal/core/sched.go:53 | s.mu.Unlock |
| `(*Scheduler).BeginTurn` | engine/internal/core/sched.go:63 | s.mu.Unlock |
| `(*TurnGuard).End` | engine/internal/core/sched.go:81 | g.s.mu.Unlock |
| `(*Scheduler).WorkshopBusy` | engine/internal/core/sched.go:88 | s.mu.Unlock |
| `(*Scheduler).WorkshopWaiting` | engine/internal/core/sched.go:95 | s.mu.Unlock |
| `(*Run).Lemmatize` | engine/internal/deepread/lemmas.go:69 | r.mu.Unlock |
| `(*Run).Err` | engine/internal/deepread/lemmas.go:99 | r.mu.Unlock |
| `(*Run).Close` | engine/internal/deepread/lemmas.go:106 | r.mu.Unlock |
| `(*Logger).Event` | engine/internal/journal/journal.go:76 | l.mu.Unlock |
| `(*Logger).Event` | engine/internal/journal/journal.go:84 | l.mu.Unlock |
| `(*Logger).Event` | engine/internal/journal/journal.go:92 | l.mu.Unlock |
| `(*Logger).OnEvent` | engine/internal/journal/journal.go:104 | l.mu.Unlock |
| `(*Client).Lemmatize` | engine/internal/lemmarpc/client.go:73 | c.mu.Unlock |
| `(*Client).Lemmatize` | engine/internal/lemmarpc/client.go:87 | c.mu.Unlock |
| `(*Client).Lemmatize` | engine/internal/lemmarpc/client.go:90 | c.mu.Unlock |
| `(*Client).Lemmatize` | engine/internal/lemmarpc/client.go:102 | c.mu.Unlock |
| `(*Client).Lemmatize` | engine/internal/lemmarpc/client.go:118 | c.mu.Unlock |
| `(*Client).Close` | engine/internal/lemmarpc/client.go:126 | c.mu.Unlock |
| `(*Client).handleReadError` | engine/internal/lemmarpc/client.go:175 | c.mu.Unlock |
| `(*Client).pump` | engine/internal/lemmarpc/client.go:200 | c.mu.Unlock |
| `(*Client).pump` | engine/internal/lemmarpc/client.go:209 | c.mu.Unlock |
| `(*progressTracker).add` | engine/internal/models/fetch.go:491 | p.mu.Unlock |
| `(*progressTracker).add` | engine/internal/models/fetch.go:495 | p.mu.Unlock |
| `(*fetchServer).ServeHTTP` | engine/internal/models/fetch_test.go:51 | s.mu.Unlock |
| `(*fetchServer).rangeHeaders` | engine/internal/models/fetch_test.go:81 | s.mu.Unlock |
| `(*OpenAIClient).LimitRemaining` | engine/internal/models/openai.go:62 | c.mu.Unlock |
| `(*OpenAIClient).setLimit` | engine/internal/models/openai.go:69 | c.mu.Unlock |
| `(*ProviderClient).LimitRemaining` | engine/internal/models/provider.go:216 | c.mu.Unlock |
| `(*ProviderClient).setLimit` | engine/internal/models/provider.go:223 | c.mu.Unlock |
| `(*Budget).ScoutSpend` | engine/internal/paganel/corridor.go:225 | b.mu.Unlock |
| `(*Budget).BuildSpend` | engine/internal/paganel/corridor.go:237 | b.mu.Unlock |
| `(*Budget).ScoutSpent` | engine/internal/paganel/corridor.go:248 | b.mu.Unlock |
| `(*Budget).BuildSpent` | engine/internal/paganel/corridor.go:255 | b.mu.Unlock |
| `(*DeepScout).Spent` | engine/internal/paganel/deep_scout.go:85 | s.mu.Unlock |
| `(*DeepScout).takeBudget` | engine/internal/paganel/deep_scout.go:122 | s.mu.Unlock |
| `(*Router).candidates` | engine/internal/providerroute/router.go:142 | r.mu.Unlock |
| `(*Router).delivered` | engine/internal/providerroute/router.go:180 | r.mu.Unlock |
| `(*Router).markFailure` | engine/internal/providerroute/router.go:188 | r.mu.Unlock |
| `(*Router).client` | engine/internal/providerroute/router.go:204 | r.mu.Unlock |
| `(*fakeClient).Stream` | engine/internal/providerroute/router_test.go:50 | f.mu.Unlock |
| `(*fakeClient).callCount` | engine/internal/providerroute/router_test.go:67 | f.mu.Unlock |
| `(*RoutedProseSlot).ProviderName` | engine/internal/providerroute/slot.go:78 | s.mu.Unlock |
| `(*RoutedProseSlot).LimitRemaining` | engine/internal/providerroute/slot.go:86 | s.mu.Unlock |
| `(*RoutedProseSlot).setMeta` | engine/internal/providerroute/slot.go:94 | s.mu.Unlock |
| `(*child).terminate` | engine/internal/supervisor/child.go:222 | c.mu.Unlock |
| `(*child).setProc` | engine/internal/supervisor/child.go:246 | c.mu.Unlock |
| `(*child).snapshot` | engine/internal/supervisor/child.go:255 | c.mu.Unlock |
| `(*Supervisor).Run` | engine/internal/supervisor/supervisor.go:125 | s.mu.Unlock |
| `(*Supervisor).Run` | engine/internal/supervisor/supervisor.go:132 | s.mu.Unlock |
| `(*Supervisor).Stop` | engine/internal/supervisor/supervisor.go:148 | s.mu.Unlock |
| `(*Supervisor).Stop` | engine/internal/supervisor/supervisor.go:154 | s.mu.Unlock |
| `(*fakeClock).Now` | engine/internal/supervisor/supervisor_test.go:32 | c.mu.Unlock |
| `(*fakeClock).After` | engine/internal/supervisor/supervisor_test.go:40 | c.mu.Unlock |
| `(*fakeRunner).Start` | engine/internal/supervisor/supervisor_test.go:101 | r.mu.Unlock |
| `(*fakeRunner).lastArgs` | engine/internal/supervisor/supervisor_test.go:108 | r.mu.Unlock |
| `(*fakeRunner).recordStop` | engine/internal/supervisor/supervisor_test.go:118 | r.mu.Unlock |
| `(*fakeRunner).startCount` | engine/internal/supervisor/supervisor_test.go:123 | r.mu.Unlock |
| `(*fakeRunner).stopOrder` | engine/internal/supervisor/supervisor_test.go:129 | r.mu.Unlock |
| `TestRestart` | engine/internal/supervisor/supervisor_test.go:169 | mu.Unlock |
| `TestRestart` | engine/internal/supervisor/supervisor_test.go:185 | mu.Unlock |
| `TestNoShortReadyBudget` | engine/internal/supervisor/supervisor_test.go:218 | mu.Unlock |
| `TestNoShortReadyBudget` | engine/internal/supervisor/supervisor_test.go:235 | mu.Unlock |
| `TestNoShortReadyBudget` | engine/internal/supervisor/supervisor_test.go:248 | mu.Unlock |
| `TestNoShortReadyBudget` | engine/internal/supervisor/supervisor_test.go:254 | mu.Unlock |
| `TestChildLogPath` | engine/internal/supervisor/supervisor_test.go:510 | runner.mu.Unlock |
| `(*Clock).Now` | engine/internal/testkit/clock.go:32 | c.mu.Unlock |
| `(*Clock).Advance` | engine/internal/testkit/clock.go:42 | c.mu.Unlock |
| `(*Clock).After` | engine/internal/testkit/clock.go:58 | c.mu.Unlock |
| `(*Clock).PendingTimers` | engine/internal/testkit/clock.go:69 | c.mu.Unlock |
| `(*FakeEmbed).Embed` | engine/internal/testkit/fakeembed.go:29 | e.mu.Unlock |
| `(*FakeEmbed).Calls` | engine/internal/testkit/fakeembed.go:36 | e.mu.Unlock |
| `(*FakeSlot).SetResponse` | engine/internal/testkit/fakeslot.go:34 | s.mu.Unlock |
| `(*FakeSlot).SetDelay` | engine/internal/testkit/fakeslot.go:42 | s.mu.Unlock |
| `(*FakeSlot).SetClock` | engine/internal/testkit/fakeslot.go:49 | s.mu.Unlock |
| `(*FakeSlot).FailWith` | engine/internal/testkit/fakeslot.go:56 | s.mu.Unlock |
| `(*FakeSlot).Calls` | engine/internal/testkit/fakeslot.go:63 | s.mu.Unlock |
| `(*FakeSlot).SetCallLimit` | engine/internal/testkit/fakeslot.go:71 | s.mu.Unlock |
| `(*FakeSlot).Generate` | engine/internal/testkit/fakeslot.go:81 | s.mu.Unlock |
| `(*FakeSlot).Generate` | engine/internal/testkit/fakeslot.go:91 | s.mu.Unlock |
| `(*SlotSpy).Generate` | engine/internal/testkit/slotspy.go:35 | s.mu.Unlock |
| `(*SlotSpy).Generate` | engine/internal/testkit/slotspy.go:41 | s.mu.Unlock |
| `(*SlotSpy).Calls` | engine/internal/testkit/slotspy.go:49 | s.mu.Unlock |
| `(*SlotSpy).Timeouts` | engine/internal/testkit/slotspy.go:56 | s.mu.Unlock |
| `(*runawayClock).Now` | engine/internal/turn/composition_test.go:284 | c.mu.Unlock |
| `(*fakeProseSlot).Stream` | engine/internal/turn/safety_test.go:27 | s.mu.Unlock |
| `(*fakeProseSlot).lastPrompt` | engine/internal/turn/safety_test.go:36 | s.mu.Unlock |
| `setActiveSlug` | engine/internal/webapi/handlers_atlas.go:73 | activeMu.Unlock |
| `getActiveSlug` | engine/internal/webapi/handlers_atlas.go:160 | activeMu.Unlock |
| `chatReturnSummary` | engine/internal/webapi/handlers_chat.go:357 | chatLastActiveMu.Unlock |
| `chatLastActiveAt` | engine/internal/webapi/handlers_chat.go:397 | chatLastActiveMu.Unlock |
| `chatTouch` | engine/internal/webapi/handlers_chat.go:405 | chatLastActiveMu.Unlock |
| `currentLlamaInstall` | engine/internal/webapi/handlers_llama_install.go:94 | llamaInstallMu.Unlock |
| `beginLlamaInstall` | engine/internal/webapi/handlers_llama_install.go:101 | llamaInstallMu.Unlock |
| `setLlamaInstallStage` | engine/internal/webapi/handlers_llama_install.go:115 | llamaInstallMu.Unlock |
| `finishLlamaInstall` | engine/internal/webapi/handlers_llama_install.go:123 | llamaInstallMu.Unlock |
| `setChannelRestartWarning` | engine/internal/webapi/handlers_settings.go:221 | channelRestartMu.Unlock |
| `getChannelRestartWarning` | engine/internal/webapi/handlers_settings.go:226 | channelRestartMu.Unlock |
| `setModelJob` | engine/internal/webapi/handlers_settings.go:1361 | modelJobsMu.Unlock |
| `modelJob` | engine/internal/webapi/handlers_settings.go:1367 | modelJobsMu.Unlock |
| `beginModelJob` | engine/internal/webapi/handlers_settings.go:1391 | modelJobsMu.Unlock |
| `handleSettingsModelStatus` | engine/internal/webapi/handlers_settings.go:1668 | modelJobsMu.Unlock |
| `newChildEmbedder` | engine/internal/webapi/handlers_settings_test.go:1282 | fe.mu.Unlock |
| `newCaptureServer` | engine/internal/webapi/telegram_commands_test.go:30 | cs.mu.Unlock |
| `(*captureServer).requests` | engine/internal/webapi/telegram_commands_test.go:42 | cs.mu.Unlock |
| `(*Hub).Close` | engine/internal/webapi/ws.go:76 | h.mu.Unlock |
| `(*Hub).Broadcast` | engine/internal/webapi/ws.go:89 | h.mu.Unlock |
| `(*Hub).add` | engine/internal/webapi/ws.go:98 | h.mu.Unlock |
| `(*Hub).remove` | engine/internal/webapi/ws.go:104 | h.mu.Unlock |
| `(*client).setTopics` | engine/internal/webapi/ws.go:212 | c.mu.Unlock |
| `(*client).send` | engine/internal/webapi/ws.go:218 | c.mu.Unlock |
| `(*client).writeFrame` | engine/internal/webapi/ws.go:279 | c.mu.Unlock |
| `(*bookMapProvider).requestBody` | engine/internal/workshop/bookmap_route_test.go:38 | p.mu.Unlock |
| `newBookMapProvider` | engine/internal/workshop/bookmap_route_test.go:52 | p.mu.Unlock |
| `(*roleCountingSlot).Generate` | engine/internal/workshop/editor_queue_test.go:26 | s.mu.Unlock |
| `(*roleCountingSlot).roleCalls` | engine/internal/workshop/editor_queue_test.go:32 | s.mu.Unlock |
| `(*PauseTimer).MarkActive` | engine/internal/workshop/pause.go:84 | t.mu.Unlock |
| `(*PauseTimer).Silence` | engine/internal/workshop/pause.go:91 | t.mu.Unlock |
| `(*recordSlot).Generate` | engine/internal/workshop/pause_test.go:84 | s.mu.Unlock |
| `(*recordSlot).Generate` | engine/internal/workshop/pause_test.go:92 | s.mu.Unlock |
| `(*Scheduler).SetSedimentRunner` | engine/internal/workshop/scheduler.go:65 | s.mu.Unlock |
| `(*Scheduler).SetChapterClosed` | engine/internal/workshop/scheduler.go:78 | s.mu.Unlock |
| `(*Scheduler).Step` | engine/internal/workshop/scheduler.go:85 | s.mu.Unlock |
| `(*Scheduler).runOne` | engine/internal/workshop/scheduler.go:179 | s.mu.Unlock |

## once_do (9)

| Function | File | Detail |
|----------|------|--------|
| `LoadData` | engine/internal/birth/ancketa.go:81 | dataOnce.Do |
| `(*Scheduler).AcquireWorkshop` | engine/internal/core/sched.go:49 | once.Do |
| `(*TurnGuard).End` | engine/internal/core/sched.go:75 | g.once.Do |
| `(*child).markReady` | engine/internal/supervisor/child.go:190 | c.readyOnce.Do |
| `(*child).stop` | engine/internal/supervisor/child.go:231 | c.stopOnce.Do |
| `(*execProcess).Stop` | engine/internal/supervisor/child.go:331 | p.stopOnce.Do |
| `(*fakeProcess).Stop` | engine/internal/supervisor/supervisor_test.go:65 | p.stopOnce.Do |
| `censorForRating` | engine/internal/workshop/censor_profile.go:155 | rating16Once.Do |
| `censorForRating` | engine/internal/workshop/censor_profile.go:158 | rating21Once.Do |

## rwmutex_lock (21)

| Function | File | Detail |
|----------|------|--------|
| `Load` | engine/internal/netgate/gate.go:96 | defaultGate.mu.Lock |
| `SetLogger` | engine/internal/netgate/gate.go:104 | defaultGate.mu.Lock |
| `(*gate).client` | engine/internal/netgate/gate.go:123 | g.mu.RLock |
| `SetSlot` | engine/internal/paganel/forge_ring.go:71 | slotMu.Lock |
| `slotForge` | engine/internal/paganel/forge_ring.go:78 | slotMu.RLock |
| `SetCensor` | engine/internal/paganel/forge_ring.go:85 | censorMu.Lock |
| `censorForge` | engine/internal/paganel/forge_ring.go:92 | censorMu.RLock |
| `SetRating` | engine/internal/workshop/censor_profile.go:90 | activeRatingMu.Lock |
| `ActiveRating` | engine/internal/workshop/censor_profile.go:97 | activeRatingMu.RLock |
| `SetLemmatizer` | engine/internal/workshop/editcensor.go:34 | editLemMu.Lock |
| `editLemmatizer` | engine/internal/workshop/editcensor.go:41 | editLemMu.RLock |
| `SetCensor` | engine/internal/workshop/horoscope.go:82 | censorMu.Lock |
| `SetEmbedder` | engine/internal/workshop/horoscope.go:89 | embedMu.Lock |
| `censorForge` | engine/internal/workshop/horoscope.go:98 | censorMu.RLock |
| `embedderForge` | engine/internal/workshop/horoscope.go:108 | embedMu.RLock |
| `SetLogger` | engine/internal/workshop/logger.go:19 | logMu.Lock |
| `loggerOf` | engine/internal/workshop/logger.go:26 | logMu.RLock |
| `SetSlot` | engine/internal/workshop/queue.go:152 | slotMu.Lock |
| `SetScheduler` | engine/internal/workshop/queue.go:160 | schedMu.Lock |
| `scheduler` | engine/internal/workshop/queue.go:167 | schedMu.RLock |
| `slotForProvider` | engine/internal/workshop/queue.go:179 | slotMu.RLock |

## rwmutex_unlock (21)

| Function | File | Detail |
|----------|------|--------|
| `Load` | engine/internal/netgate/gate.go:98 | defaultGate.mu.Unlock |
| `SetLogger` | engine/internal/netgate/gate.go:106 | defaultGate.mu.Unlock |
| `(*gate).client` | engine/internal/netgate/gate.go:126 | g.mu.RUnlock |
| `SetSlot` | engine/internal/paganel/forge_ring.go:72 | slotMu.Unlock |
| `slotForge` | engine/internal/paganel/forge_ring.go:79 | slotMu.RUnlock |
| `SetCensor` | engine/internal/paganel/forge_ring.go:86 | censorMu.Unlock |
| `censorForge` | engine/internal/paganel/forge_ring.go:93 | censorMu.RUnlock |
| `SetRating` | engine/internal/workshop/censor_profile.go:92 | activeRatingMu.Unlock |
| `ActiveRating` | engine/internal/workshop/censor_profile.go:98 | activeRatingMu.RUnlock |
| `SetLemmatizer` | engine/internal/workshop/editcensor.go:35 | editLemMu.Unlock |
| `editLemmatizer` | engine/internal/workshop/editcensor.go:42 | editLemMu.RUnlock |
| `SetCensor` | engine/internal/workshop/horoscope.go:83 | censorMu.Unlock |
| `SetEmbedder` | engine/internal/workshop/horoscope.go:90 | embedMu.Unlock |
| `censorForge` | engine/internal/workshop/horoscope.go:99 | censorMu.RUnlock |
| `embedderForge` | engine/internal/workshop/horoscope.go:109 | embedMu.RUnlock |
| `SetLogger` | engine/internal/workshop/logger.go:20 | logMu.Unlock |
| `loggerOf` | engine/internal/workshop/logger.go:27 | logMu.RUnlock |
| `SetSlot` | engine/internal/workshop/queue.go:153 | slotMu.Unlock |
| `SetScheduler` | engine/internal/workshop/queue.go:161 | schedMu.Unlock |
| `scheduler` | engine/internal/workshop/queue.go:168 | schedMu.RUnlock |
| `slotForProvider` | engine/internal/workshop/queue.go:180 | slotMu.RUnlock |

## waitgroup_add (3)

| Function | File | Detail |
|----------|------|--------|
| `main` | Levant/generate_SVG_map.go:92 | wg.Add |
| `DownloadFile` | engine/internal/models/fetch.go:182 | wg.Add |
| `(*Supervisor).Run` | engine/internal/supervisor/supervisor.go:134 | s.wg.Add |

## waitgroup_wait (3)

| Function | File | Detail |
|----------|------|--------|
| `main` | Levant/generate_SVG_map.go:106 | wg.Wait |
| `DownloadFile` | engine/internal/models/fetch.go:191 | wg.Wait |
| `(*Supervisor).Stop` | engine/internal/supervisor/supervisor.go:159 | s.wg.Wait |

