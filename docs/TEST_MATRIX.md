# Проверки HomeQuest — 2026-11-16

Команды: `make test`, `make harness`; `go test -race ./internal/homequest/bot
./internal/homequest/task ./internal/homequest/reward ./internal/homequest/wishlist`.
В Cloud добавлять `GO=/workspace/toolchains/go/bin/go` в make. Все перечисленные
unit/integration/Bot fixtures проходили. Последний запуск/версия — STATUS/RUNBOOK.

| ID | Существующие проверки | Уровень / предел доказательства |
|---|---|---|
| E01 | bot.TestE01ConversationAndReplay; family/service_test.go | Bot controlled transport + SQLite; A/B/C/D, приглашения/изоляция; часть invite setup через доменный сервис |
| E02 | tests/bot-widget-e02.mts; task.TestCompletionRaceSnapshotAndAuthorization | Реальный HTML → официальный QuickJS/renderer gesture → Bot.Handle → SQLite → новый state; баланс/вклад/XP; 9 width/theme; race отдельно integration |
| E03 | bot.TestE03PhotoApprovalThroughUpdates; task.TestPhotoApprovalSnapshotRejectAndUndo; TestConcurrentApproveRejectIsSingleDecision | Bot E2E controlled transport + integration; фото/approval/отказ/снимок/undo/гонка |
| E04 | chore.TestHomeQuestLateAcceptanceAndDST; task.TestLateApprovalCreatesOnlyFutureInstance; TestTimezoneChangePreservesLocalScheduleAndInvalidatesCards; bot.TestScheduleDialogGrammar | Unit + SQLite + grammar; несколько пропущенных недель, DST, timezone. Полный bot→worker календарный E2E не собран |
| E05 | bot.TestReminderPlanningRestartSnoozeAndInvalidation; TestQuietHoursAndVacation | SQLite/outbox с fake clock; отдельные quiet/vacation правила. Не настоящий Telegram delivery |
| E06 | reward.TestRedemptionReservationRefundSnapshotAndUndo; bot.TestCatalogsAndPremiumThroughUpdates | Integration race/snapshot/refund/undo + product updates request→approve→deliver |
| E07 | wishlist.TestSecretClaimsRaceIsolationAndDeparture; bot.TestCatalogsAndPremiumThroughUpdates | Concurrent claim, неизменный owner DTO, чужая семья, уход + UI flow. API отдельного сайта не существует |
| E08 | assistant.TestSafeCommandValidation; bot.TestE08E09AIUpdatesVoiceDraftAndReplay | STT/LLM fixtures через очередь и product updates; отрицание/чужой ID/replay. Реальная LLM проверена отдельно; human voice не доказан |
| E09 | тот же TestE08E09; assistant schema test | Черновик не создаёт instance до подтверждения, повтор кнопки без дубля; расписание во family timezone |
| E10 | billing.TestManualEntitlementAuthorizationModeAndPersistence; bot.TestCatalogsAndPremiumThroughUpdates | Серверный mode/role/scope, expiry/revoke/restart + product buttons. Реальных payment методов нет |
| E11 | family/service_test.go; task auth/photo tests; wishlist isolation; bot.TestOldTaskActionAfterRuleChangeIsRejected; task.TestDesignatedReviewerAndDeparture; assistant foreign ID | Unit/integration + отдельные bot stale actions; не единый полный adversarial E2E всех сущностей |
| E12 | bot runtime_test.go; widget.TestTransportRedactsErrors; TestStatusIsStructuredWithoutDescription; assistant schema/fixtures | 429/backoff, 409 stop, slow send independent ack, dedup, JSON. Часть STT/LLM отказов не имеет отдельного end-to-end теста |
| E13 | task.TestLegacyAttemptMigration; bot.TestDialogueBackAndRestartState; stale action test; tests/card-layouts.mts | Реальная SQLite migration, restart диалогов, старые версии; официальный renderer размеров/тем. Не покрывает все возможные старые БД |

Upstream baseline: Donetick v0.1.86 до изменений запускался на пустой SQLite,
повторный запуск успешен; `go test ./...` проходил. Новые проверки не скрывают
baseline failures — их не обнаружено.

Официальный harness: pinned SHA в REUSE; spike 9 layouts + E02 9 layouts;
продуктовые карточки из настоящих handlers проверяются на 200px, state ≤1024,
height ≤600; 3 обезличенных screenshots. E02 использует контролируемый host/API,
но настоящий QuickJS, validator и renderer. Обычный browser mock не используется.

## Live smoke

- getMe/getWebhookInfo: HTTP 200, @Home_questbot, webhook отсутствует.
- Получен реальный update в личном чате. sendWidget: HTTP 400
  INPUT_MEDIA_WIDGET_NOT_ALLOWED, несмотря на private chat. Fallback переводит ту же
  доставку в sendMessage с полезным текстом и inline buttons. Это не live Widgets PASS.
- Новый разрешённый VyceAI: /models и completion HTTP 200; точный ID
  deepseek-4.1-flash-1017. TestLiveVyceAISemantics: 4/4 PASS через реальный Go client.
  TestLiveGatewayCompletionThroughBot: PASS, реальная LLM + product handlers + SQLite,
  Telegram transport controlled. JSON Schema strict принят; локальная валидация остаётся.
- faster-whisper small CPU int8 реально загружен и запущен. Синтетическая русская
  речь распознана с искажением смысла/времени глагола: качество **не принято**.
  Первый замер 9.94 s, peak RSS 1,048,704 KiB. Живое человеческое voice ожидается.
- При заблокированной LLM бот может показать только локальную транскрипцию;
  это не считается завершением задания голосом и не меняет баллы.

Opt-in команда: HOMEQUEST_LIVE_AI=1 HOMEQUEST_SECRETS_FILE=/absolute/private/secrets.json
 go test ./internal/homequest/assistant ./internal/homequest/bot -run TestLive -count=1 -v.
В обычном CI live tests SKIP. TestConsentIsBoundToProvider и gateway configuration
тесты проходят без внешних запросов. Реальный человеческий voice остаётся открытым.

Новый live-дефект: INPUT_MEDIA_WIDGET_NOT_ALLOWED воспроизведён на настоящем API.
TestWidgetDenialExposesOnlyKnownReason и TestWidgetDeniedDeliversUsefulButtons PASS:
типизированная причина без raw ошибки/токена, один fallback, сохранение кнопок,
без повторного доменного эффекта. Не считать fallback доказательством Widgets.

Аудит 16 ноября: make test PASS (cache), make harness PASS повторно:
9 spike + 9 E02 + 17 product cards, 3 screenshots. Повторный getMe/webhook/models
HTTP 200. Новых Telegram Widgets или human-voice PASS нет.
Причина запрета подтверждена ответом levlam от 13 ноября в
[официальном issue #947](https://github.com/tdlib/telegram-bot-api/issues/947):
обычным ботам запрещены Widgets даже в private chats. Руководство по доступности
не отражает это ограничение. Полный список незакрытых критериев — STATUS.
