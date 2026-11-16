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
  deepseek-4.1-flash-1017. TestLiveVyceAISemantics: 8/8 PASS через реальный Go client.
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

Известное внешнее ограничение: INPUT_MEDIA_WIDGET_NOT_ALLOWED воспроизведён на настоящем API.
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

Дополнительные проверки этого изменения: TestSwitchUIWithoutChangingActions,
TestGatewayRejectionHasSafeStatus, TestMultipleHomesThroughButtons,
TestOtherMembersTaskCardRefreshesThroughOutbox (включая stale retry после навигации),
TestAssistantFailureKeepsManualCompletion — PASS. Race: bot/family/task/reward/wishlist
PASS. Миграция отдельной командой два раза на одной пустой SQLite — PASS.
Тесты сначала воспроизводили отсутствие переключения домов, обновлений карточек,
награды-достижения и зависание ACK; после реализации проходят.

widget-feedback.mts проверяет ACK без state, rate limit, timeout, failure и
переход на вторую страницу действий без серверного request. card-layouts.mts
дополнен длинным русским названием и font 20 на 320/390px. Эти проверки выявили
переполнение 600px; карточка теперь разбивает длинный список действий на страницы.

Итог 2026-11-16 07:12 UTC: `make test harness GO=/workspace/toolchains/go/bin/go
WIDGET_SANDBOX=/workspace/widget-sandbox` — exit 0. Все перечисленные новые
проверки и browser bridge PASS. `getMe/getWebhookInfo/models` HTTP 200, webhook
отсутствует. Новая live Widgets/human voice приёмка не заявляется.

CI первого HTML checkpoint выявил другое поведение системного шрифта: длинная
карточка 320px/font20 достигала 628px. Gate не ослаблен. Длинные заголовки получили
компактную шапку, текст и действия — локальную пагинацию. Добавлена проверка
200px/font20 и доступности последней части текста через реальный runtime gesture.
После исправления шрифтов полный локальный `make harness` повторно PASS: все
compatibility/E02/product layouts, включая 200px/font20, feedback/pagination и
browser bridge. Go backend с прошлого зелёного `make test` не менялся.

Deployment preparation: `python3 -m unittest discover -s tests/ops -v` — PASS:
archive whitelist/links/path traversal/checksums/commit, права при umask 077,
успешная активация, migration rollback до старта и сохранение accepted updates
при readiness failure. SQLite и filesystem настоящие временные; systemctl,
migration и health в этих tests контролируемые. Это не live server deployment.
`bash -n ops/deploy-from-ci.sh` PASS. Проверка systemd unit внутри Cloud упёрлась
в read-only /run/systemd; фактическая установка/применение лимитов пока не проверены.
Дополнительно PASS: отказ до остановки сервиса при нехватке диска и ограниченная
retention, не удаляющая посторонние пути. Всего 9 ops tests. Реальный упакованный
CGO-disabled homequest-migrate выполнен на пустой SQLite и повторно на той же БД:
оба запуска PASS. Архив ~136 MB, только явный whitelist; секретов/БД в нём нет.

Новые feature gates: TestFeatureCardBudgets создаёт реальные карточки через handlers
с максимальными кириллическими полями и URL; make harness передаёт их в официальный
renderer. TestGiftSecrecyAcrossProductPayloads использует A/B/C/D, конкурентные
Bot.Handle, неизменность разрешённого автору state/подробностей и фактический вход
контролируемого LLM adapter. Сырые данные дарителя не передаются этому adapter.
TestFamilyReminderPlanThroughButtonsAndRestart проверяет invalidation старых jobs
и snooze, сохранение сроков, смену политики владельцем и новые сообщения worker.
403 tests проверяют отсутствие backlog, восстановление свежим private update и
гонку запоздалого отказа с новым /start. Это controlled transport, не live Telegram.

Feature checkpoint 2026-11-16: `make test harness` exit 0; Go tests всех пакетов,
9 spike + 9 linked E02 layouts, 32 product cards, 5 official-renderer screenshots,
feedback/pagination и расширенный browser path — PASS. `go test -race
./internal/homequest/bot ./internal/homequest/wishlist` — PASS. Реальные
Telegram Widgets/напоминания/человеческий голос этим запуском не проверялись.
