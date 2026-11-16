# HomeQuest

Разработка Telegram-бота для семейного быта на базе Donetick v0.1.86.
**Работает ранняя пробная версия: семьи, простые задания, фото и подтверждение.**
/start создаёт дом; есть приглашения, Widgets, баллы и undo. Полный MVP ещё не готов:
повторения, напоминания, награды, wishlist, AI/STT и Premium остаются в работе.
Состояние и точка продолжения: [docs/STATUS.md](docs/STATUS.md).

## Локальная проверка без Docker

Нужны Go 1.24.3 (toolchain может скачать требуемую зависимостями версию), Node 24,
Python 3, npm, corepack, Git. В этой среде Go: /workspace/toolchains/go/bin/go.

```sh
make build test
make widgets
./scripts/setup-widget-harness.sh
make harness
DATA_DIR=/absolute/path/outside/repository/homequest-data make dev
```

Upstream backend слушает порт 2021. Health: `curl -f http://localhost:2021/api/v1/health`.
Миграции запускаются синхронно при старте; `make migrate` запускает backend с ними.
Это внутренний backend, не готовый пользовательский интерфейс HomeQuest.

Настройки см. .env.example; scripts/dev.sh не загружает .env автоматически.
Задайте переменные окружения или используйте секретное хранилище вне checkout.
Для безопасного preflight: `python3 scripts/preflight.py /path/outside/git/secrets.json`.
Не копируйте исходный промпт или реальные ключи в репозиторий.

[Происхождение](docs/REUSE.md) · [План](docs/PLAN.md) · [Тесты](docs/TEST_MATRIX.md) ·
[Дизайн](docs/DESIGN.md) · [Запуск и перенос](docs/RUNBOOK.md).
Исходная лицензия Donetick сохранена в LICENSE.md.

Запуск Telegram runner после `make build`:

```sh
DATA_DIR=/absolute/private/data HOMEQUEST_SECRETS_FILE=/absolute/private/secrets.json ./bin/homequest
```

Проверка: `curl -f http://127.0.0.1:2022/health`. Использует ту же `donetick.db` в DATA_DIR.
