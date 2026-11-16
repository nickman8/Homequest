# Запуск, остановка, перенос

Временная среда: checkout `/workspace/Homequest`, данные
`/workspace/homequest-private/runtime/donetick.db`, секреты
`/workspace/homequest-private/secrets.json` (0600), лог `bot.log` в private-каталоге.
Никакие из этих приватных файлов не входят в Git. SQLite backup перед последним
обновлением: `/workspace/homequest-private/pre-upgrade-e41b7b6.db`.

Проверять health при каждом возобновлении. Код обновляется после тестов, с backup
и остановкой только собственного polling consumer. Доступность после завершения
Cloud-сессии не гарантирована.

```sh
cd /workspace/Homequest
make widgets build GO=/workspace/toolchains/go/bin/go
DATA_DIR=/workspace/homequest-private/runtime \
HOMEQUEST_SECRETS_FILE=/workspace/homequest-private/secrets.json \
BILLING_MODE=manual_test \
TELEGRAM_UI=buttons \
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
LLM/STT, выбранный UI и scheduler heartbeat (ready/delayed/error). «Configured» не означает успешный live AI smoke. Миграции GORM
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
исключения. VyceAI endpoint и проверенный model ID заданы в приватном конфиге через прежние
NICKLLM_* имена. Для smoke использовать scripts/preflight.py и opt-in TestLive
из TEST_MATRIX. При смене gateway согласие на обработку запрашивается снова.

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

При точном Telegram INPUT_MEDIA_WIDGET_NOT_ALLOWED outbox сохраняет fallback
в sendMessage с теми же actions; health показывает widget_fallbacks. Это ограничение
сервера Widgets, не успех их live-проверки и не блокер HTML-разработки. Не устранять его бесконечными
повторами. Статические шаблоны/официальный harness сохранены для доступных клиентов.

HTML preview: `make preview GO=/workspace/toolchains/go/bin/go`, затем
http://127.0.0.1:4173. Это loopback-стенд с временной SQLite и вымышленными A/B/C/D.
Он не публичная ссылка для телефона. Остановка Ctrl-C удаляет его временную БД.
После включения Widgets у Telegram: TELEGRAM_UI=widgets и обычный restart;
пересоздание доменных данных не требуется.

Отдельная миграция: остановить бот, выполнить `DATA_DIR=/absolute/private/data
make migrate GO=/workspace/toolchains/go/bin/go`. Она завершается после миграций,
не обращается в Telegram и не запускает workers. Повтор безопасен.

Проверено 2026-11-16 07:12 UTC: запущен код e41b7b6; health: database=ready,
telegram=polling, scheduler=ready, ui=buttons. LLM/STT configured; live-проверки
отдельно в TEST_MATRIX. Процесс Cloud session 93316; это не systemd service.
HTML preview session 70428, loopback 4173, HTTP 200.
