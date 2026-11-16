# Запуск, остановка, перенос

Временная среда: checkout `/workspace/Homequest`, данные
`/workspace/homequest-private/runtime/donetick.db`, секреты
`/workspace/homequest-private/secrets.json` (0600), лог `bot.log` в private-каталоге.
Никакие из этих приватных файлов не входят в Git. SQLite backup перед последним
обновлением: `/workspace/homequest-private/pre-upgrade-5c58982.db`.

```sh
cd /workspace/Homequest
make widgets build GO=/workspace/toolchains/go/bin/go
DATA_DIR=/workspace/homequest-private/runtime \
HOMEQUEST_SECRETS_FILE=/workspace/homequest-private/secrets.json \
BILLING_MODE=manual_test \
STT_PYTHON=/workspace/homequest-private/stt-venv/bin/python \
STT_MODEL_DIR=/workspace/homequest-private/stt-models \
./bin/homequest
```

Остановка: Ctrl-C для foreground либо SIGTERM конкретному своему процессу.
Дождаться выхода; затем повторить команду. Не применять killall/не останавливать
неизвестный deployment. Локальный flock DATA_DIR/polling.lock запрещает второй
consumer. Telegram 409 останавливает polling. Webhook проверяется и не удаляется;
pending updates не сбрасываются. Cursor/receipts сохраняются в той же БД.

Health: `curl -f http://127.0.0.1:2022/health`. Показывает DB/polling/настройку
LLM/STT и worker. «Configured» не означает успешный live AI smoke. Миграции GORM
синхронны при старте; запуск на той же БД идемпотентен. Не публиковать внутренний
Donetick backend 2021 без отдельного аудита. Для polling он не требуется.

Напоминания: REMINDER_OFFSETS_MINUTES=-60,0,120. При пропуске более 5 минут старое
уведомление отбрасывается. Quiet hours/snooze изменяют доставку, не срок задания.
Отпуск отключает личную доставку. Retry bounded; потерянный ответ Telegram может
дать повтор сообщения, но не повтор начисления/списания. После паузы не ждать
доставки всех накопленных уведомлений.

AI: очередь до 16, один worker, лимиты 100/500 в сутки через config. Сначала
согласие; text/file ID удаляются после обработки/15 минут. Voice до 60s/4MiB,
STT timeout 90s; локальное аудио удаляется после запуска. Не выводить raw HTTP
исключения. NickLLM 403 — повторить preflight из разрешённой провайдером среды;
не обходить блокировку, не угадывать model ID.

BILLING_MODE=manual_test разрешает владельцу тестовый Premium. В других режимах
ручная выдача отклоняется сервером. Stars только будущий интерфейс, платежей нет.

## Backup / restore

```sh
python3 scripts/backup.py /absolute/private/data/donetick.db /absolute/private/backup.db
```

Для restore остановить процесс, сохранить прежнюю БД, скопировать backup тем же
скриптом в новый DATA_DIR/donetick.db, проверить запуск. Не перезаписывать живую БД.
Хранить защищённую внешнюю копию; текущая локальная копия не защищает от потери VM.

Перенос: checkout зафиксированного commit, pinned зависимости, make test/harness,
защищённый перенос БД/конфига, один service manager process. Проверить webhook и
что старый polling остановлен. Исходящий HTTPS; публичный endpoint не нужен.
Codex Cloud не гарантирует жизнь процесса после завершения работы; nohup этого
не меняет. Здесь не используются keepalive-задачи для продления среды.
