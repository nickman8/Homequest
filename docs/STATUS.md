# Статус — 2026-11-16

Полная приёмка MVP ещё не завершена; полезная пробная версия запущена.

## Реализованные сценарии

- Дом/приглашения/участники, owner transfer, выход, изоляция семей.
- Задания, назначения, редактирование цены/названия, срок, отмена/пауза.
- Фото/approval независимы, конкретный проверяющий, аудит reviewed_by/at,
  повторная попытка после отказа, снимок цены, undo прямого выполнения 5 минут.
  Если проверяющий вышел, принять может другой текущий взрослый, кроме исполнителя.
- Повторы daily/weekdays/interval/после принятия; accepted_at, DST, один актуальный
  экземпляр; смена timezone сохраняет местное время повторов. Разовые UTC-сроки
  и история не сдвигаются. Старые карточки/jobs инвалидируются версией.
- Durable outbox, before/due/after reminders, quiet hours, отпуск, snooze;
  независимые polling/delivery/AI workers. Нет обещания exactly-once Telegram send.
- Ledger, история с performed/accepted датами, баланс/неделя/XP, равные места,
  6 достижений, общий дом. Расходы наград не уменьшают вклад/XP.
- Награды с атомарным списанием при запросе, снимком имени/цены, принятием,
  выдачей, отказом/отменой/refund. После выдачи отмены с возвратом нет.
- Wishlist: обычный URL, заметка/цена, атомарная тайная бронь/покупка;
  автор не получает claim-поля. Owner может освободить брони ушедших без раскрытия IDs.
- Manual-test Premium, серверная проверка режима/owner, отдельное entitlement.
- AI/STT adapters и очередь: согласие, минимальный task-only контекст,
  ограниченный JSON, проверка IDs/версий/прав, подтверждение черновика/нескольких дел,
  контролируемые fixtures. VyceAI подключён с проверенным model ID deepseek-4.1-flash-1017.

## Проверки и ограничения

`go test ./...`, Bot E2E с controlled transport, официальный QuickJS/renderer
E02 (9 layouts) + compatibility spike (9), 17 продуктовых layouts и screenshots.
Тесты покрывают гонки выполнения/approval/расходов/брони, снимки цен, изоляцию,
старые права, DST/позднее approval, restart/retry, schema validation, миграции.
Полные E01–E13 на одном сквозном уровне пока не доказаны; подробности TEST_MATRIX.

Telegram getMe: @Home_questbot, webhook отсутствует. Получен первый реальный update в личном чате.
Telegram sendWidget отвечает INPUT_MEDIA_WIDGET_NOT_ALLOWED; добавлен fallback
на обычные сообщения с теми же серверными действиями; первая fallback-доставка
подтверждена настоящим Telegram API. Live Widgets пока заблокированы. По разрешению владельца gateway переключён
на https://vyceai.com/v1: /models и completion HTTP 200, точный ID
`deepseek-4.1-flash-1017`. JSON Schema accepted; реальный Go client прошёл 4
семантических проверки и отдельный bot→real LLM→SQLite→одно начисление сценарий.
Согласие привязано к gateway: прежнее согласие не переносится на новый автоматически.
Прежние ошибки NickLLM 403/1010 и no_db_connection остаются историей диагностики,
но больше не блокируют этот deployment. Старый сервер не изменялся.

Повторная диагностика Widgets: в [официальном Bot API issue #947](https://github.com/tdlib/telegram-bot-api/issues/947)
levlam 13 ноября 2026 подтвердил: обычным ботам Widgets запрещены даже в личных
чатах. [Руководство](https://core.telegram.org/bots/widgets#availability) всё ещё
обещает private production; это расхождение источников, а не настройка BotFather.
Наш минимальный HTML без JS/state тоже получил этот отказ. Формат POST JSON,
plain UTF-8 html, private chat, allow_queries и widget_query сверены с документацией.
family.html: 1594 байта UTF-8 / 891 байт Python zlib9; NUL отсутствует.
Обход ограничения не реализуется. Срок восстановления и публичный порядок
допуска в прочитанном обсуждении не опубликованы. Test-server live не проверялся.

Faster-whisper small CPU int8 выполнен на синтетическом русском аудио; качество
не прошло смысловой критерий (искажение глагола). Живая человеческая запись
вместе с Telegram/NickLLM-совместимым API нового gateway ещё не проверена.

Остаток продуктовой приёмки: полная визуальная проверка редких/длинных состояний, дополнительные связи E04–E13,
live Telegram/Widgets/русский voice. Лимит карточек больше 100 снят; для
крупных семей позже нужна SQL-пагинация вместо загрузки списка целиком.

## Среда и продолжение

DATA_DIR=/workspace/homequest-private/runtime; секреты вне Git с правами 0600.
Пробный процесс работает временно; точная версия/рестарт — RUNBOOK. Uptime Cloud
не гарантирован, локальная БД не является надёжной удалённой резервной копией.

Ветка codex/homequest-mvp опубликована через Git Data API; API credentials
работают, HTTPS Git — нет. Новый PAT не создан и не нужен для публикации.
Force:false, tree equality и secret scan. Draft PR #1 создан в codex/homequest-baseline (review-only bootstrap); main/master не менялись.

Продолжать с текущего checkout, без повторного выбора базы: make test/harness,
затем реальный /start и голосовой smoke. Opt-in live проверки провайдера:
HOMEQUEST_LIVE_AI=1 HOMEQUEST_SECRETS_FILE=/absolute/private/secrets.json
 go test ./internal/homequest/assistant ./internal/homequest/bot -run TestLive -count=1 -v.
В CI эти тесты пропускаются; семейные данные и ключи в fixtures не используются.

## Аудит всех T00–T14

Проверены код текущего checkout, тесты и официальный API/runtime. Реализация и
live-приёмка — разные статусы. Непроверенный критерий не считается выполненным.

| ID | Реализовано | Что отсутствует / не принято |
|---|---|---|
| T00 | Осмотр checkout/инструкций/skills, запуск Donetick до изменений, SHA доноров, документация Widgets, реальные getMe/webhook/models/completion, выбранная модель | Первоначальная оценка доступности Widgets исправлена по issue #947. SSH/LiteLLM сервер не проверен и не изменён: прямой TCP недоступен из среды; для VyceAI он больше не нужен |
| T01 | Go/GORM/SQLite upstream, миграции на пустой и существующей БД, lockfiles, сборка без Docker, test/dev/CI, безопасный конфиг, health, backup script | Полноценные структурированные логи с correlation ID отсутствуют. Health AI/STT показывает configured, не текущую работоспособность; scheduler без отдельного heartbeat. make migrate — alias запуска dev, отдельной завершающейся команды миграций нет |
| T02 | sendWidget/editMessageWidgetData/answerWidgetQuery, allowed_updates, привязанные серверные actions, byte budgets, официальный QuickJS/validator/renderer, E02 | Реальные отправка/нажатие/обновление Widgets заблокированы Telegram; test-server live не выполнен. Не все исходы request отражаются в UI: успешный ACK без state update может оставить кнопку disabled |
| T03 | Создание дома с IANA timezone, Telegram ID, invite 7 дней/hash/revoke/idempotency, участники, передача owner/исключение/выход, scope и старые права | Два реальных человека общий дом ещё не прошли. Переключатель между существующими членствами upstream отсутствует: UI допускает один дом, Join другого требует выхода; полное сохранение multi-membership UX не выполнено |
| T04 | Общий пул/назначение, создание, цена/название/описание/срок, редактирование, отмена, сохранение диалога, атомарное выполнение и replay/race protection | Нет автоматического обновления уже открытых карточек остальных участников; свежие данные доступны при новом действии/открытии. Полный live путь создания/выполнения не проверен |
| T05 | Независимые photo/approval, optional photo, отдельные attempts/instances, конкретный reviewer и замена после выхода, запрет self-approval, reject/resubmit, snapshot, undo 5 минут с компенсацией/проверкой баланса, sendPhoto с авторизацией | Фото и approval в настоящем Telegram-клиенте ещё не проверены; текущие проверки используют контролируемый transport |
| T06 | daily/дни недели/N дней/после принятия, accepted_at, один просроченный instance, пауза/перенос/timezone/DST, durable reminders/outbox, retry/restart, quiet hours/mute/vacation/snooze | Нет единого bot→scheduler E04 через несколько недель; реальная доставка напоминаний не принята. Нет пользовательского статуса недоступной доставки. Интервалы уведомлений меняются в конфиге, не отдельным семейным UI |
| T07 | Единая история баланса, отдельные неделя/XP, Monday timezone, равные места, снимки цен/компенсации, уровень дома, 6 достижений | Нет достижения «первая полученная награда» и категорий дел; вместо категорий есть вклад в 3 разных дня. Цель 100 баллов фиксирована. Live-проверки рейтинга/экономики нет |
| T08 | Каталог/create/edit/archive (имя, описание, цена), request со списанием и snapshot, approve/deliver, reject/cancel/refund один раз, защита баланса и undo | Отдельный regression test архивирования каталога с ожидающей заявкой отсутствует; live-выдача не проверена |
| T09 | Желание с URL/заметкой/ценой, family scope, атомарная тайная бронь, покупка/снятие, DTO без claim для автора, cleanup ушедшего | Редактирование/удаление желания в UI отсутствуют (не отдельный критерий T09, но ограничение продукта). Нет одного сквозного теста всех каналов утечки: widget state/уведомления/AI проверяются не единой цепочкой. Live-сюрприз не проверен |
| T10 | VyceAI с подтверждённым deepseek-4.1-flash-1017, JSON Schema + локальная валидация, согласие на gateway, минимальный контекст, очередь/лимиты/timeouts, STT adapter, безопасные completion/draft/уточнение/undo; реальная LLM прошла 4 семантических случая и bot→LLM→SQLite | Смысловое качество STT на синтетической русской фразе не принято. Человеческое Telegram voice→STT→LLM→действие не проверено. Полный набор неоднозначных фраз проверен fixtures, не live-eval |
| T11 | Manual-test entitlement, owner/mode/scope/expiry/revoke, сохранение, расширенная статистика/AI лимит, отдельные интерфейсы будущих платежей | Подписка/Stars и обработчик реальных payment events отсутствуют намеренно; есть модель/контракт, не готовая платёжная интеграция. Live-переключение Premium не проверено |
| T12 | Русские экраны, общий шаблон, Telegram theme, Open Props tokens, одна Lucide house, кнопки/назад/отмена, pagination, 17 карточек и 3 screenshots официального renderer | Остальные задуманные иконки/специализированные компоненты не сделаны. Часть ввода требует строки с разделителями или IANA ID. Все редкие состояния/длинные строки/крупный шрифт и действия целиком не приняты |
| T13 | Unit/integration/Bot tests, E02 связанный официальный harness, отдельные E01–E13 проверки, CI, миграция legacy attempts, stale cards | Не все E01–E13 собраны как полные пути через продуктовые handlers. Нет живого клиентского E2E. Отдельные проверки отказа STT/LLM, всей изоляции и всех старых схем БД остаются |
| T14 | Polling/flock/cursor, webhook guard/409 stop, workers, runbook/start/stop/backup, опубликованная ветка и draft PR #1 | Процесс временный: при повторном аудите прежний процесс уже отсутствовал. Постоянного service manager/сервера и внешнего backup нет; доступность после завершения Cloud не обещается. Полная приёмка зависит от T02/T10/T12/T13 |

На этом аудите `make test` PASS (Go cache использован), `make harness` PASS:
9 compatibility + 9 E02 layouts + 17 product cards; 3 screenshots.
Первый запуск тестов внутри sandbox упёрся в read-only Go cache; повтор с
разрешённым доступом прошёл. Это ограничение среды, не падение теста приложения.
Повторный getMe/getWebhookInfo/models — HTTP 200; webhook отсутствует.
Существующие live AI результаты не заменены новыми mock-прогонами.

После проверки отсутствия другого локального consumer и webhook пробный процесс
перезапущен из bin/homequest (код f54f780). Readiness подтверждён: database=ready,
telegram=polling, llm/stt=configured, scheduler=polling_worker. Последнее поле —
название режима, не самостоятельный heartbeat. Это состояние на момент проверки,
не гарантия работы после завершения Cloud-сессии.
