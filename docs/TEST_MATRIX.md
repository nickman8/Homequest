# Проверки

2026-11-16:
- Неизменённый upstream: `go test ./...` PASS, 31 пакет с тестами; `go build` PASS.
- Импортированный backend: `make build test` PASS (32 пакета с тестами).
- `go test ./internal/homequest/widget`: PASS. До реализации тесты были красными
  (отсутствовали NewClient/Validate/NewSpike). Байтовые лимиты кириллицы, редактирование
  ошибок transport, другой viewer, истечение action, повторный запрос, реальная SQLite.
- `npm run build:widgets`: PASS, 1392/813 bytes HTML/zlib9.
- `tests/widget-harness.mts`: PASS, 9 комбинаций ширин 200/320/390 и light/dark/custom.
  Настоящие QuickJS + validator + renderer с нажатием → Go CLI controlled transport →
  SQLite → изменённый chatData → проверка текста в QuickJS. Это T02 spike, не E02.
  Family scope и task effects в нём отсутствуют. Нет подставленного успешного результата.
- Upstream миграции на пустой БД и повторном старте: PASS, health HTTP 200.
- Live Telegram: только getMe/getWebhookInfo PASS; настоящая карточка/клик не проверены.
- NickLLM: models HTTP 403; completion/structured outputs не проверены.
- STT: не установлен; русское аудио не проверено. CI workflow подготовлен, remote run не подтверждён.

| Сценарий | Тест продукта | Статус |
|---|---|---|
| E01 семья/вступление | отсутствует | Не реализован |
| E02 выполнение | отсутствует; T02 spike не заменяет | Не реализован |
| E03 фото/approval | отсутствует | Не реализован |
| E04 повтор/DST | upstream baseline не доказывает accepted_at | Не реализован |
| E05 reminders | upstream baseline не доказывает HomeQuest | Не реализован |
| E06 награда/refund | upstream тесты; другие правила списания | Не реализован |
| E07 секретная бронь | отсутствует | Не реализован |
| E08 голос | отсутствует | Не реализован |
| E09 AI draft | отсутствует | Не реализован |
| E10 Premium | отсутствует | Не реализован |
| E11 семейная изоляция | только viewer-bound probe | Не реализован |
| E12 восстановление | только redaction transport | Не реализован |
| E13 миграции/старые карточки | baseline миграции и размер одной карточки | Частично |

Первый импорт содержит исходные whitespace замечания Donetick (`git diff --check`
на root commit). Upstream файлы ради форматирования не переписывались.
Проверка новых файлов на whitespace и скан реальных переданных секретов — PASS.

## Новая проверка вертикальных сценариев

- family/service_test.go: две семьи, expires/revoke, repeat join, owner leave, transfer,
  removal, timezone, запрет неявного переключения дома — PASS.
- task/service_test.go: concurrent completion, назначение, edit authorization — PASS.
- task/approval_test.go: required photo, self-approval, snapshot, reject/resubmit,
  duplicate approval/undo, foreign photo, migration предыдущей БД — PASS.
- bot/bot_test.go: E01 через updates, duplicate receipt, чужой action, stale version — PASS.
- bot/photo_test.go: E03 через updates с controlled transport — PASS.
- bot/runtime_test.go: durable bounded retry/429, отсутствие повторной доставки после
  успеха, 409 stop — PASS. Не полноценная проверка всех сетевых сбоев.
- tests/bot-widget-e02.mts: официальный QuickJS/renderer → обычный bot handler → SQLite
  → widget state; 9 тем/ширин — PASS. Проверены баланс и повтор; недельный вклад/XP пока нет.

Предыдущая таблица фиксировала первоначальный checkpoint. E01/E03 теперь покрыты указанными
путями; E02 покрыт в базовой части. E04–E10 (кроме фото-ветки), полные E11–E13 ещё не завершены.

## Дополнительный запуск T06–T11 (2026-11-16)

`go test ./...` PASS после добавления расписания, напоминаний, экономики,
наград, wishlist и Premium. `make harness` PASS: 9 spike + 9 E02 layouts.

| Требование | Существующая проверка | Уровень / результат |
|---|---|---|
| E04 accepted_at, позднее approval, DST | task.TestLateApprovalCreatesOnlyFutureInstance; chore homequest_schedule_test | unit + SQLite integration PASS |
| E04 timezone / старые версии | task.TestTimezoneChangePreservesLocalScheduleAndInvalidatesCards | SQLite integration PASS |
| E05 планирование, quiet/vacation/snooze, restart | bot/reminders_test.go | SQLite + fake clock PASS |
| E06 снимки, гонка расхода/refund/undo | reward.TestRedemptionReservationRefundSnapshotAndUndo | integration PASS |
| E07 бронь, автор/чужая семья/уход | wishlist.TestSecretClaimsRaceIsolationAndDeparture | integration PASS |
| E10 режим, роль, persistence, expiry | billing.TestManualEntitlementAuthorizationModeAndPersistence | integration PASS |
| E06/E07/E10 кнопки/диалоги | bot.TestCatalogsAndPremiumThroughUpdates | Bot E2E controlled transport PASS |
| E12 медленный Telegram send не задерживает query ack | bot.TestSlowDeliveryDoesNotBlockWidgetAcknowledgement | controlled transport PASS |

Live Widgets, живой русский голос и NickLLM completion не подтверждены.
Полная матрица E01–E13 пока не закрыта; тест отдельного правила не равен полному E2E.
