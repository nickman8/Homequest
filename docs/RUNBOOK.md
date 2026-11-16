# Запуск / остановка / перенос

Ранний Telegram runner: `DATA_DIR=/workspace/homequest-private/runtime HOMEQUEST_SECRETS_FILE=/workspace/homequest-private/secrets.json ./bin/homequest`.
Health: `curl -f http://127.0.0.1:2022/health`. Остановить SIGTERM, дождаться выхода и повторить
команду. flock в DATA_DIR защищает от второго локального consumer; Telegram 409 останавливает
процесс. Webhook проверяется и не удаляется. Jobs напоминаний ещё не реализованы.

```sh
cd /workspace/Homequest
make build GO=/workspace/toolchains/go/bin/go
DATA_DIR=/workspace/homequest-private/runtime ./scripts/dev.sh
curl -f http://localhost:2021/api/v1/health
```

Остановить foreground процесс Ctrl-C/SIGTERM; рестарт той же командой.
Миграции штатные, синхронные при запуске. Health upstream подтверждает HTTP,
но не readiness polling/LLM/STT. Не запускать два экземпляра на одном порту/БД.
Среда может требовать sandbox escalation для listening socket и сетевых запросов.

Согласованная копия SQLite (данные НЕ коммитить):
`python3 scripts/backup.py "$DATA_DIR/donetick.db" /secure/location/backup.db`.
Для restore остановить backend, сохранить старую БД, затем той же командой скопировать
backup.db в новый путь donetick.db. Сохранить отдельно jwt-secret и uploads, если появятся.
Сначала проверить восстановленную копию на отдельном DATA_DIR.

Перенос: checkout зафиксированного commit, установить Go/Node, make build/test,
скопировать закрытые данные и секреты по защищённому каналу, запустить под service manager.
Не публиковать upstream backend без отдельной настройки доступа/TLS.
Будущий polling: сначала getWebhookInfo, не удалять чужой webhook/updates;
один consumer, durable offset/outbox, при 409 остановить конфликтующий запуск.

Codex Cloud не гарантирует жизнь процесса после завершения задачи. nohup не даёт uptime.

Напоминания: `REMINDER_OFFSETS_MINUTES=-60,0,120` (до, в срок, один повтор).
После пропуска более 5 минут старое уведомление отбрасывается. Quiet hours
переносят только доставку. Планировщик и один outbox worker работают независимо
от polling; потерянный ответ Telegram всё ещё может дать повтор сообщения.
`BILLING_MODE=manual_test` разрешает владельцу тестовое entitlement; остальные
значения запрещают ручную выдачу на сервере. Живых платежей нет.
