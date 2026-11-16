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
| E08 | assistant.TestSafeCommandValidation; bot.TestE08E09AIUpdatesVoiceDraftAndReplay | STT/LLM fixtures через очередь и product updates; отрицание/чужой ID/replay. Live speech/LLM НЕ доказаны |
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
- Polling и DB health проверены. Настоящий Telegram-клиент, карточка и callback
  ожидают пользовательского `/start`; найденные чаты не использовались для рассылки.
- NickLLM GET /v1/models: HTTP 403, model ID неизвестен. Completion/structured outputs
  не проверены, другая модель не подставлена.
- faster-whisper small CPU int8 реально загружен и запущен. Синтетическая русская
  речь распознана с искажением смысла/времени глагола: качество **не принято**.
  Первый замер 9.94 s, peak RSS 1,048,704 KiB. Живое человеческое voice ожидается.
- При заблокированной LLM бот может показать только локальную транскрипцию;
  это не считается завершением задания голосом и не меняет баллы.
