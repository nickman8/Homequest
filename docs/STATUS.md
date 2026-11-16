# Состояние разработки — 2026-11-16

MVP ещё не завершён. Реализован первый пользовательский путь в Telegram.

## Уже работает

- /start → создать дом → IANA timezone → пригласить участников ссылкой на 7 дней.
- Upstream users/circles, серверная проверка членства, отзыв приглашений, выход,
  передача управления, исключение участника, изменение timezone.
- Создание одноразового общего задания: название и баллы через диалог; выполнение
  из Widget. Старые кнопки привязаны к viewer/family/instance/version/TTL.
- Фото и approval как независимые правила, отдельные попытки, снимок цены,
  запрет self-approval, отклонение и повторная подача. Фото через sendPhoto, без URL.
- Прямое выполнение: компенсация undo в течение 5 минут при достаточном балансе.
- Durable update receipts/outbox, ограниченные retries, остановка при Telegram 409,
  локальный flock одного consumer. Потерянный ответ Telegram может повторить сообщение,
  но не начисление. Проверка текущего членства перед доставкой.

## Проверено

Go unit/integration и bot flows на SQLite; официальный QuickJS/validator/renderer:
настоящее нажатие → основной обработчик → БД → новое отображение, 200/320/390px,
light/dark/custom. E02 базовое выполнение; вклад недели/XP ещё не реализованы.
E03 фото/approval проходит через продуктовые updates с controlled Telegram transport.
Миграция ранней схемы attempts проверена отдельным тестом.

## Остаётся

T03–T05 имеют работающие вертикальные сценарии, но не все критерии: UI назначения,
редактирования/отмены задания, конкретный проверяющий, дополнительные гонки и recovery.
T06 повторения/напоминания, T07 XP/рейтинг/достижения, T08 награды, T09 wishlist,
T10 AI/голос, T11 Premium, T12 полировка, полная T13 матрица — не готовы.
Видимый текст продукта сообщает о незавершённых возможностях.

## Внешние интеграции и запуск

Бот @Home_questbot подтверждён getMe, webhook отсутствует. Polling запущен;
health http://127.0.0.1:2022/health различает Telegram/DB/неподключённые LLM/STT/scheduler.
Проверка реальной карточки и нажатия ожидает пользовательского /start; harness не считается live.
NickLLM models HTTP 403; точный ID DeepSeek 4.1 Flash неизвестен, completion не подменён.
STT не установлен. Это не блокирует обычные действия.

DATA_DIR=/workspace/homequest-private/runtime; секреты в
/workspace/homequest-private/secrets.json (0600), вне Git. Лог bot.log там же.
Срок жизни Cloud процесса не гарантируется; БД не является надёжной резервной копией.

## Git

Ветка codex/homequest-mvp. Существующий GH_TOKEN работает для GitHub API, но не для
Git HTTPS. Выпуск PAT через integration API не предоставлен; deploy keys возвращают 403.
Новый токен не потребовался: публикация snapshot через Git Data API с force:false,
проверкой tree SHA и сканированием секретов. main/master не создаются и не меняются.
Remote API commit отличается от local source commit; оба сохраняются в сообщении.
Пока единственная ветка не имеет отдельной базы для draft PR.

## Продолжение

1. make test GO=/workspace/toolchains/go/bin/go
2. make harness GO=/workspace/toolchains/go/bin/go WIDGET_SANDBOX=/workspace/widget-sandbox
3. Завершить T03–T05 и T06 с повторным использованием internal/chore/scheduler.go;
   обязательно accepted_at и поздний approval/DST, затем durable reminders.
4. Довести оставшиеся T07–T14; статус не заменяет исходные критерии владельца.
