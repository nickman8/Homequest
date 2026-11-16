# Проверки HomeQuest — 2026-11-16

Команды: `make test`, `make harness`; `go test -race ./internal/homequest/bot
./internal/homequest/task ./internal/homequest/reward ./internal/homequest/wishlist`.
В Cloud добавлять `GO=/workspace/toolchains/go/bin/go` в make. Все перечисленные
unit/integration/Bot fixtures проходили. Последний запуск/версия — STATUS/RUNBOOK.

| ID | Существующие проверки | Уровень / предел доказательства |
|---|---|---|
| E01 | bot.TestE01ConversationAndReplay; family/service_test.go; bot.TestMultipleHomesThroughButtons | Bot controlled transport + SQLite; A/B/C/D, приглашения/изоляция; часть invite setup через доменный сервис |
| E02 | tests/bot-widget-e02.mts; task.TestCompletionRaceSnapshotAndAuthorization; bot.TestOtherMembersTaskCardRefreshesThroughOutbox | Реальный HTML → официальный QuickJS/renderer gesture → Bot.Handle → SQLite → новый state; баланс/вклад/XP; 9 width/theme; race отдельно integration |
| E03 | bot.TestE03PhotoApprovalThroughUpdates; task.TestPhotoApprovalSnapshotRejectAndUndo; TestConcurrentApproveRejectIsSingleDecision | Bot E2E controlled transport + integration; фото/approval/отказ/снимок/undo/гонка |
| E04 | chore.TestHomeQuestLateAcceptanceAndDST; task.TestLateApprovalCreatesOnlyFutureInstance; TestTimezoneChangePreservesLocalScheduleAndInvalidatesCards; bot.TestScheduleDialogGrammar; bot.TestE04LateApprovalThroughBotAndWorker | Unit + SQLite + Bot/worker с fake clock: несколько пропущенных недель, DST, timezone, restart, позднее approval и старые callbacks |
| E05 | bot.TestReminderPlanningRestartSnoozeAndInvalidation; TestQuietHoursAndVacation; TestFamilyReminderPlanThroughButtonsAndRestart | SQLite/outbox с fake clock; отдельные quiet/vacation правила. Не настоящий Telegram delivery |
| E06 | reward.TestRedemptionReservationRefundSnapshotAndUndo; bot.TestCatalogsAndPremiumThroughUpdates | Integration race/snapshot/refund/undo/archive + product updates request→approve→deliver→первая награда badge |
| E07 | wishlist.TestSecretClaimsRaceIsolationAndDeparture; bot.TestCatalogsAndPremiumThroughUpdates; TestGiftSecrecyAcrossProductPayloads; TestWishWizardEditArchiveAndStaleButtons; wishlist.TestOwnerEditsArchiveAndSecretClaims; TestConcurrentEditUsesOneVersion | Concurrent claim, неизменный owner DTO, чужая семья, уход + UI flow. API отдельного сайта не существует |
| E08 | assistant.TestSafeCommandValidation; bot.TestE08E09AIUpdatesVoiceDraftAndReplay | STT/LLM fixtures через очередь и product updates; отрицание/чужой ID/replay. Реальная LLM проверена отдельно; human voice не доказан |
| E09 | тот же TestE08E09; assistant schema test | Черновик не создаёт instance до подтверждения, повтор кнопки без дубля; расписание во family timezone |
| E10 | billing.TestManualEntitlementAuthorizationModeAndPersistence; bot.TestCatalogsAndPremiumThroughUpdates | Серверный mode/role/scope, expiry/revoke/restart + product buttons. Реальных payment методов нет |
| E11 | family/service_test.go; task auth/photo tests; wishlist isolation; bot.TestOldTaskActionAfterRuleChangeIsRejected; task.TestDesignatedReviewerAndDeparture; assistant foreign ID | Unit/integration + отдельные bot stale actions; не единый полный adversarial E2E всех сущностей |
| E12 | bot runtime_test.go; widget.TestTransportRedactsErrors; TestStatusIsStructuredWithoutDescription; bot.TestDeliveryUnavailableAndPrivateChatRecovery; TestOldDeliveryFailureCannotBlockNewPrivateUpdate; assistant schema/fixtures; bot.TestAssistantFailureKeepsManualCompletion | 429/backoff, 409 stop, slow send independent ack, dedup, JSON. STT failure/LLM timeout/invalid JSON проходят очередь и затем ручное выполнение |
| E13 | task.TestLegacyAttemptMigration; wishlist.TestLegacyWishlistMigration; bot.TestDialogueBackAndRestartState; stale action test; tests/card-layouts.mts | Реальная SQLite migration, restart диалогов, старые версии; официальный renderer размеров/тем. Не покрывает все возможные старые БД |

Upstream baseline: Donetick v0.1.86 до изменений запускался на пустой SQLite,
повторный запуск успешен; `go test ./...` проходил. Новые проверки не скрывают
baseline failures — их не обнаружено.

Официальный harness: pinned SHA в REUSE; spike 9 layouts + E02 9 layouts;
продуктовые карточки из настоящих handlers проверяются на 200px, state ≤1024,
height ≤600; 5 обезличенных screenshots. E02 использует контролируемый host/API,
но настоящий QuickJS, validator и renderer. Совместимость не доказывается обычным browser mock. Отдельный tests/preview.mts
проверяет browser bridge → handlers → SQLite: дом, join, task, completion, balance, создание/правка/архив желания и семейные интервалы.
Это разработческий стенд, не проверка Telegram-клиента.

## Live smoke

- getMe/getWebhookInfo: HTTP 200, @Home_questbot, webhook отсутствует.
- Получен реальный update в личном чате. sendWidget: HTTP 400
  INPUT_MEDIA_WIDGET_NOT_ALLOWED, несмотря на private chat. Fallback переводит ту же
  доставку в sendMessage с полезным текстом и inline buttons. Это не live Widgets PASS.
- Новый разрешённый VyceAI: /models и completion HTTP 200; точный ID
  deepseek-4.1-flash-1017. TestLiveVyceAISemantics: 10/10 PASS через реальный Go client.
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

## Подробные HTML E2E

Каждый сценарий идёт через реальные кнопки HTML, controlled Telegram updates,
Bot.Handle, миграции и временную SQLite. Ни один успешный доменный результат не
подставляется прямой записью из теста. Отдельный процесс bot-probe на каждом
update дополнительно проверяет сохранение состояния после restart. Для worker
используются управляемые часы, для AI — явно обозначенные adapters/fixtures.

| Файл в tests/ | Покрытые пути и необычные случаи |
|---|---|
| preview.mts | E01/E02, создание/edit/archive желания, семейные интервалы, темы |
| household-e2e.mts | E01/E03/E06/E07/E10/E11: invalid timezone, фото→reject→новое фото→approval, self/foreign/replay, snapshot награды после изменения цены, insufficient balance, refund once, запрет refund после выдачи, гонка тайной брони и неизменный owner payload, XSS, pause/resume/cancel, исключение/передача владельца/отозванное приглашение/выход |
| plans-e2e.mts | Набор: invalid points/edit/skip/duplicate; комнаты: canonical duplicate/rename/delete с сохранением задач; очередь и добровольная передача/отказ/replay/чужой токен/старое выполнение; недельная граница, цель, mute и реальный outbox worker без повторной рассылки |
| shopping-e2e.mts | Batch/draft/edit/claim race/buy/reopen/archive, чужая семья, старая отменённая кнопка, двойное сохранение, max Cyrillic/XSS/invalid length; fixture voice→consent→реальная очередь→черновик→HTML confirm/cancel без ранних записей |
| assistant-e2e.mts | E08/E09 + undo: fixture voice, согласие, завершение, компенсация/replay, ошибочная команда модели на отрицание/будущее/неуверенность/прошлое, чужой ID, task draft cancel/confirm/replay, optional photo, истёкшее undo, рейтинг/достижения/история |
| e2e-runtime.mts | Все различные state из четырёх подробных путей → официальный QuickJS/validator/renderer, 200px, 1024-byte state/600px height; новые главные экраны также 320/390 dark/custom |
| bot-widget-e02.mts | Отдельно связанный официальный runtime gesture → сформированный request → handlers/SQLite → state → новый renderer; 9 width/theme комбинаций |
| widget-harness.mts, widget-feedback.mts, card-layouts.mts | Spike и общий HTML, ACK/timeout/rate-limit/error, локальная пагинация, длинный текст, 200px/font20, 32 граничные карточки |

Это покрытие всех продуктовых HTML-шаблонов и основных разделов, а не всех
комбинаторных сочетаний данных. Network 429/409, миграции и календарный DST
дополнительно проверяются на Bot/integration уровне; живой Telegram не симулируется
под видом успешного клиентского теста.

## Доменные регрессии расширения

- task.TestRoomsPresetsAtomicScopeAndHistory — транзакция набора/rollback, чужая семья, история.
- task.TestRotationAcceptanceDepartureAndHandoffRace — очередь только после позднего approval, гонка решений.
- task.TestRotationUndoRestoresOriginalTurn — восстановление предыдущего исполнителя.
- task.TestRotationSkipsDepartedCurrentAssignee — уход не оставляет дело заблокированным.
- task.TestRotationCannotScheduleSelfReviewerAndStaleHandoff,
  TestReviewerCannotBecomeFutureRotatingPerformer — конфликт будущего исполнителя/проверяющего в обоих порядках настройки.
- task.TestExpiredHandoffPreservesAssignment — 7-дневный срок.
- bot.TestHandoffNotificationAndClosedOfferInvalidation — личная durable доставка, закрытое предложение не отправляется.
- bot.TestWeeklyGoalSnapshotsAndDigestRestart, TestWeeklyGoalSurvivesTimezoneChange, TestWeeklyGoalForAnUnvisitedEmptyWeek — Monday snapshot, restart/no backlog, timezone не переписывает цель; пустая неделя без запусков сохраняет действовавшую цель.
- shopping.TestShoppingAtomicBatchClaimVersionsAndScope — атомарный batch/гонка/version/audit/уход/restart.
- assistant.TestShoppingDraftValidation — ограниченная schema, без чужих ID и смешанных команд.

TDD воспроизвёл дефекты undo очереди, ушедшего исполнителя, изменения цели при
timezone и конфликта проверяющего; исправления подтверждены зелёными регрессиями.

Ops: 9 проверок archive/checksum/path traversal, activation/rollback/retention/disk
через python3 -m unittest discover -s tests/ops -v. systemctl/health контролируются;
реального server deployment эти тесты не доказывают.


Последний локальный полный make test/harness: PASS (16 ноября).
HTML journeys: 174 + 101 + 80 + 43 = 398 состояний; официальный renderer:
123 различные проекции на 200px + 16 новых экран/тема/ширина.
Дополнительно прежние 9 spike, 9 связанных E02 и 32 граничные карточки PASS.
Race bot/task/stats/shopping PASS. После уточнения политики пустых недель
Go suite/build и plans HTML повторены. Live VyceAI eval 10/10 PASS (42.71 s);
этот результат не распространяется на STT или настоящий Telegram-клиент.
