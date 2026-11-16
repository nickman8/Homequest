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
