# HomeQuest

Пробный Telegram-бот для семейного быта на базе **Donetick v0.1.86**, Go/GORM/SQLite.
Бот: [@Home_questbot](https://t.me/Home_questbot). Доступность временного Cloud-процесса
не гарантируется. `/start` → создать дом → выбрать IANA timezone → добавить задание.

Работают дома и приглашения, задания/фото/подтверждение/undo, повторы и напоминания,
баланс/вклад/XP/достижения, семейные награды, секретный wishlist, тестовый Premium.
Основной UI — компактные Telegram Widgets; ввод — обычные сообщения.
Сейчас Telegram отклоняет Widgets для тестового чата (`INPUT_MEDIA_WIDGET_NOT_ALLOWED`),
поэтому бот автоматически доставляет обычные сообщения с кнопками.

**AI подключён через VyceAI**: `https://vyceai.com/v1`, проверенный ID
`deepseek-4.1-flash-1017`. Реальные проверки JSON Schema, выполнения, отрицания,
будущего действия и черновика расписания прошли. Полная приёмка MVP ещё не
завершена: живой Telegram-клиент и человеческое русское голосовое ожидают проверки.
Локальный faster-whisper работает; синтетический STT smoke не подтвердил качество.

## Установка без Docker

Go 1.24.3, Node 24, Python 3, npm, corepack, Git. В текущей Cloud-среде Go находится
в `/workspace/toolchains/go/bin/go`; можно передать `GO=...` в make.

```sh
make widgets build test
./scripts/setup-widget-harness.sh
make harness
```

Настройки: [.env.example](.env.example). Экспортируйте переменные в процесс;
автоматическая загрузка `.env` не выполняется. Секреты допускаются в JSON-файле
вне checkout с правами 0600; значения из окружения имеют приоритет.

```sh
python3 scripts/preflight.py /absolute/private/secrets.json
DATA_DIR=/absolute/private/data HOMEQUEST_SECRETS_FILE=/absolute/private/secrets.json BILLING_MODE=manual_test ./bin/homequest
curl -f http://127.0.0.1:2022/health
```

Миграции GORM запускаются синхронно, SQLite `donetick.db` хранится в DATA_DIR.
Второй локальный consumer блокируется flock; webhook не удаляется; Telegram 409
останавливает polling. Нужны исходящие HTTPS-запросы, публичный endpoint не нужен.
`make dev` запускает исходный внутренний Donetick backend на 2021; семье он не нужен.
Не запускайте его как альтернативный публичный интерфейс без отдельного аудита.

## Локальное распознавание речи

```sh
python3 -m venv /absolute/private/stt-venv
/absolute/private/stt-venv/bin/pip install -r requirements-stt.lock
STT_MODEL_DIR=/absolute/private/stt-models /absolute/private/stt-venv/bin/python scripts/setup-stt.py
```

В окружении бота задайте STT_PYTHON, STT_MODEL_DIR и STT_MODEL=small.
CPU int8, 2 потока, одна обработка одновременно, до 60 секунд/4 МБ.
Для совместимости сохранён один набор настроек NICKLLM_BASE_URL/API_KEY/MODEL;
он теперь указывает на разрешённый владельцем VyceAI. Другой модели в fallback нет.
Аудио временное; текст AI-очереди удаляется после обработки или через 15 минут.

[Статус](docs/STATUS.md) · [Происхождение](docs/REUSE.md) · [План](docs/PLAN.md) ·
[Матрица тестов](docs/TEST_MATRIX.md) · [Дизайн](docs/DESIGN.md) · [Runbook](docs/RUNBOOK.md).
Upstream LICENSE/NOTICE сохранены. БД, реальные ключи и исходное секретное задание
не должны попадать в Git, CI или публичные отчёты.
