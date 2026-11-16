# Повторное использование — проверено 2026-11-16

## Выбранная база

[Donetick](https://github.com/donetick/donetick), tag v0.1.86,
SHA `ebfb340d7bd4e2e009d6f2f9ad073868acaf6412`.
Импортирован весь отслеживаемый исходный код через git archive, включая go.mod/go.sum,
миграции, GORM/SQLite, circles/users, chores/scheduler, notifier/planner, points,
reward/redemption, LICENSE.md. Стек Go сохранён. Исходный README перенесён в
UPSTREAM_README.md; upstream workflows сохранены в upstream-workflows, но не запускаются
автоматически (включали scheduled release). Новая логика только в internal/homequest,
cmd/widget-probe, widgets и scripts/tests. Семейный Telegram-адаптер пока не реализован.

До изменений: go test ./... — PASS (31 пакет с тестами); go build — PASS.
Запуск оригинального бинарника на пустой SQLite выполнил миграции; sandbox запрещал
listening socket, после разрешённого запуска вне sandbox health HTTP 200.
Повторный запуск на той же БД также успешен. Upstream checkout остался чистым.

Проверенные ограничения (не исправлены):
- chore/handler.go ApproveChore допускает admin/manager, не любого другого взрослого;
- следующий срок при approval вычисляется от pendingHistory.PerformedAt, не accepted_at;
- reward/repo сохраняет snapshot названия/цены, но списывает при approval без резерва
  при запросе; нет отдельного состояния выдачи/отмены с refund;
- текущие Task/History/points нельзя считать уже подходящим ledger для HomeQuest;
- Telegram user mapping, фото-права и независимая попытка/экземпляр требуют отдельной проверки.
База пригодна для адаптации; отказа от Donetick нет.

## Реально использованные зависимости

| Источник | Пин | Что используется |
|---|---|---|
| TelegramMessenger/widget-sandbox | `1cde0068daba8437f53eac0b61c395d29b8f89e3` (package 0.6.5) | неизменённый sandbox-node, QuickJS, validator, Chromium renderer, frozen pnpm lock |
| Open Props | npm 1.7.23 + package-lock integrity | exports sizes/borders: size-2, size-3, radius-3 извлекаются при сборке |
| Lucide Static | npm 1.73.0 + package-lock integrity | icons/house.svg inline; лицензия сохранена |

README sandbox описывает старый SDK без запросов; исходники и runtime-проверка
подтвердили request/chatDataChanged. Пин обязателен. В production JS-библиотеки не входят.

## Изученные, но ещё не перенесённые доноры

- cmintey/wishlist `ee56cb9635a96a594140a95d57bc3e37322a750b`:
  src/lib/server/list.ts, routes/lists/[id]/+page.server.ts, config.ts.
  Есть group membership и опциональная видимость claims владельцу. Не считать её
  доказательством требуемого отсутствия скрытых полей в payload; нужна отдельная DTO-проверка.
- finalbillybong/ChoreQuest `0523d5ea4ef36a26b0cc60d2d33b09173f9e0dc0`:
  backend/routers/vacation.py использует require_parent и общую выборку отпусков
  без family scope; напрямую для мультисемейного HomeQuest непригоден.
- SYSTRAN/faster-whisper HEAD `6cb639e70786aea42b9233006ae60eaf07190bdb`:
  только подтверждён доступный SHA; пакет/модель ещё не установлены, reuse не заявляется.

## Инструкции и источники

AGENTS.md в checkout и просмотренных донорах не найдены.
Прочитан skill cloud-environment-runtime (package c4), SKILL.md и references
networking.md, repositories.md, packages.md: сохранение checkout, обычные HTTPS проверки,
различение service denial и sandbox, установка в пользовательскую директорию.
Специализированных Telegram/TDD/security skills в доступном каталоге нет.
Прочитаны core.telegram.org/bots/widgets.md (включая Testing/Writing widgets with an LLM),
Bot API index и методы sendWidget/editMessageWidgetData/WidgetQuery, официальный runtime profile.

## Вертикальный адаптер HomeQuest

internal/homequest/family использует непосредственно upstream User/Circle/UserCircle;
internal/homequest/task использует Chore, ChoreHistory, PointsHistory. Добавлены только
Telegram identity, timezone settings, invitations, instance/attempt/rules и bot receipts/
actions/outbox в той же SQLite. Баланс вычисляется из PointsHistory; UserCircle.Points
обновляется транзакционно как upstream projection. Existing external API/admin frontend
не используются как общий admin token. Main bot runner не открывает payment endpoints.

Рассмотрен gstack: https://github.com/garrytan/gstack,
SHA `8c3c39b5220f7fb0c53af5362863a2377f756279`.
Прочитаны README, verify/SKILL.md и релевантные части review/SKILL.md/checklist.md.
Полная установка/setup и прогон gstack не выполнялись, новые навыки не объявляются
установленными. Использованы как ориентир отдельные проверки transactional boundaries,
stale authorization, migrations и сохранения regression gates. Найден и исправлен
stale task action после изменения цены, с красным → зелёным regression test.
Официальный Telegram harness остаётся доказательством совместимости Widgets.
