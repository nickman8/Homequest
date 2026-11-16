# Статус — 2026-11-16

Пробная версия реализована на Donetick; полная live-приёмка ещё не завершена.
Запрет Telegram Widgets по решению владельца не блокирует разработку: основной
HTML проверяется в официальном runtime и отдельном браузерном стенде. Сейчас
бот работает обычными кнопками (`TELEGRAM_UI=buttons`). Для Widgets достаточно
сменить режим на `widgets` и перезапустить: шаблон и backend остаются теми же.

## Полный аудит T00–T14

«Реализовано» описывает код и автоматические проверки, а не успех живого клиента.

| ID | Реализовано и проверено | Остаток / ограничение |
|---|---|---|
| T00 | Проверены checkout/инструкции/skills/доноры; исходный Donetick запущен до изменений; SHA и переносы в REUSE; getMe/webhook/models/completion | SSH/LiteLLM сервер не проверен и не изменён: TCP недоступен. Для разрешённого VyceAI он не требуется |
| T01 | Go/GORM/SQLite, lockfiles, запуск без Docker, CI, отдельная завершающаяся команда migrate, повтор миграций, backup, JSON-логи с update/delivery IDs, scheduler heartbeat | Health AI/STT сообщает configured, а не непрерывный live smoke. Нет внешнего долговременного backup |
| T02 | Настоящие sendWidget/editMessageWidgetData/answerWidgetQuery, byte budgets, персональные actions, официальный QuickJS/validator/renderer, E02; HTML preview с реальными handlers/SQLite; ACK/error feedback | Live Widgets запрещены Telegram обычным ботам; test-server не проверен. Browser bridge отдельно обозначен и не считается Telegram-клиентом |
| T03 | Telegram ID, IANA timezone, случайные 7-дневные hash invitations/revoke/idempotency, роли/передача owner/выход, сохранение нескольких членств и явный выбор дома без скрытой смены контекста | Два реальных человека пока не прошли весь путь совместного дома |
| T04 | Общий пул/назначение, создание, описание/цена/срок, редактирование/отмена, сохранение диалога; атомарное выполнение, replay/race; обновление последней открытой карточки других участников | Автообновление реализовано для Widget-карточек дома/заданий/баллов/рейтинга/прогресса/достижений; обычные сообщения обновляются при открытии. Live создание→выполнение не принято |
| T05 | Отдельные attempts/instances, независимые optional/required photo и approval, reviewer с заменой после выхода, запрет self-approval, reject/retry, снимок цены, undo 5 минут с компенсацией/проверкой свободного баланса, scoped sendPhoto | Фото и approval в настоящем Telegram-клиенте ещё не проверены |
| T06 | daily/weekdays/interval/after acceptance, accepted_at, один просроченный экземпляр, DST/timezone/пауза/перенос; durable reminders, quiet/mute/vacation/snooze; связанный bot→worker E04 с поздним approval через недели и DST | Живая доставка напоминаний не принята; отдельного UI статуса недоступной доставки нет. Интервалы уведомлений меняются конфигом |
| T07 | Единый ledger, баланс/неделя/XP, Monday timezone, равные места, snapshots/compensation, общий дом, 7 достижений включая первую выданную награду | Категорий дел нет: заменены вкладом в 3 разных дня. Недельная цель 100 фиксирована. Live рейтинг/экономика не приняты |
| T08 | Каталог/create/edit/archive, списание при request, snapshot, approve/deliver/reject/cancel/refund один раз; проверены гонки и архивирование с ожидающей заявкой | Live-выдача награды не проверена |
| T09 | Название/URL/заметка/информационная цена, семейная изоляция, атомарная тайная бронь/покупка/снятие; DTO не выдаёт автору claim; cleanup ушедшего | Редактирование/удаление желания в UI отсутствуют. Нет единого E2E всех каналов утечки; live-сюрприз не проверен |
| T10 | VyceAI, проверенный deepseek-4.1-flash-1017, JSON Schema + локальная проверка, согласие на gateway, task-only контекст, очередь/лимиты, STT adapter, completion/draft/уточнения/undo; 8 реальных семантических LLM случаев PASS | Синтетическое русское STT не прошло смысловой критерий. Человеческое Telegram voice→STT→LLM→действие ожидает записи; качество голоса не объявляется принятым |
| T11 | Manual-test entitlement с owner/mode/scope/expiry/revoke и сохранением, отдельные BillingProvider/Entitlement интерфейсы, AI лимиты/расширенная статистика | Stars и реальные платежи намеренно не подключены; только модель/контракт будущих событий. Live Premium не проверен |
| T12 | Общий HTML/CSS/JS, Open Props, 8 Lucide SVG, Telegram темы, русские экраны, навигация; narrow pagination; официальный renderer и браузерные screenshots | Некоторые диалоги требуют разделителей или IANA ID. Полный перебор редких состояний и живого клиента не выполнен |
| T13 | Unit/integration/Bot tests, официальный связанный E02, E04 bot→worker, browser E2E, race checks, миграции/stale actions, CI; отказы STT/LLM не ломают ручное выполнение | Не все E01–E13 собраны в единый полный клиентский E2E. Не проверены все возможные исторические БД; точные уровни в TEST_MATRIX |
| T14 | Polling/flock/cursor/webhook guard/409 stop, раздельные workers, health, start/stop/backup, ветка/draft PR; временная среда | Постоянного сервера/service manager и внешнего backup нет. Cloud uptime не гарантирован. Состояние процесса проверять по RUNBOOK |

## Внешние проверки

Telegram: подтверждён @Home_questbot, webhook отсутствовал; настоящий update и
доставка обычного сообщения подтверждены. Минимальный `<p>Дом</p>` и продуктовый
Widget оба получили `INPUT_MEDIA_WIDGET_NOT_ALLOWED`. В
[Bot API issue #947](https://github.com/tdlib/telegram-bot-api/issues/947) levlam
13 ноября 2026 подтвердил запрет обычным ботам даже в private chat. Руководство
widgets#availability всё ещё обещает private production. Публичного срока или
документированной настройки включения нет. Обход ограничения не предпринимается.

VyceAI: /models и completion HTTP 200; модель `deepseek-4.1-flash-1017` подтверждена.
Последний расширенный live-eval: 8/8 PASS (37.85 s): completion, negation, future,
intention, uncertainty, past, multiple, Tuesday draft. Первоначальный запуск того
же eval получил gateway rejection; обычная повторная проверка и rerun прошли.
Причина временного отказа не установлена; ошибки теперь сохраняют только HTTP
status, без ответа/ключей. Более ранний bot→real LLM→SQLite smoke также PASS.

Faster-whisper small CPU int8 реально установлен и запускался, но синтетическое
русское аудио распознано с изменением смысла глагола. Нужен реальный человеческий
voice; mock и синтез не заменяют эту проверку.

## Продолжение

Данные и секреты вне Git, пути в RUNBOOK. Git HTTPS credentials не работают,
Git Data API работает; существующего доступа хватает, новый PAT не создавался.
Публикация проверяет secret scan и равенство деревьев, force:false. Draft PR #1
направлен в review-only codex/homequest-baseline; main/master не меняются.

Следующая live-проверка после readiness: /start → дом → задание → выполнение;
затем человеческое русское голосовое до 60 секунд. Пока Widgets выключены,
все независимые проверки выполняются через HTML/официальный runtime/обычные кнопки.

Последний локальный gate (2026-11-16 07:12 UTC): `make test harness` PASS; 9 compatibility +
9 связанных E02 layouts + 17 product cards + длинные строки/крупный шрифт +
ACK/error/pagination + browser bridge. Race bot/family/task/reward/wishlist PASS.
Readiness после обновления кода e41b7b6: database=ready, telegram=polling,
scheduler=ready, ui=buttons, llm/stt=configured. HTML preview HTTP 200 на 4173.
Это состояние на момент проверки, а не гарантия uptime.
