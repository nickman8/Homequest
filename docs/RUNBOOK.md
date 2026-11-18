# Запуск, остановка, перенос

Временная среда: checkout `/workspace/Homequest`, данные
`/workspace/homequest-private/runtime/donetick.db`, секреты
`/workspace/homequest-private/secrets.json` (0600), лог `bot.log` в private-каталоге.
Никакие из этих приватных файлов не входят в Git. SQLite backup перед последним
обновлением: `/workspace/homequest-private/runtime/before-agent-v3-20261118.db`.

Проверять health при каждом возобновлении. Код обновляется после тестов, с backup
и остановкой только собственного polling consumer. Доступность после завершения
Cloud-сессии не гарантирована.

```sh
cd /workspace/Homequest
make widgets build GO=/workspace/toolchains/go/bin/go
DATA_DIR=/workspace/homequest-private/runtime \
HOMEQUEST_SECRETS_FILE=/workspace/homequest-private/secrets.json \
BILLING_MODE=manual_test \
AI_MODE=agent \
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
из TEST_MATRIX. При смене gateway или scope передаваемых данных согласие запрашивается снова.
Текущий scope `agent_v3` включает комнаты, отчёты, покупки, пожелания без броней, награды, баллы и статистику. Queued-запросы со старым
scope не отправляются расширенному агенту; пользователь повторяет просьбу.

`AI_MODE=agent` включает готовый Eino runtime: максимум 6 model iterations,
12 tool calls и 75 секунд на запрос (25 секунд на HTTP-вызов). Текст/voice используют
одну bounded очередь. Сессии hq_agent_sessions и планы hq_agent_plans находятся
в той же SQLite; backup включает их. Планы и их содержимое истекают через 15 минут.
Контекст/до 4 обращений — после 24 часов без активности; ручной сброс доступен
в /agent. При рестарте ещё актуальный запрос агента с текущим согласием может начать планирование заново один раз. Старые и повторно прерванные запросы очищаются. Уже показанный план можно подтвердить до истечения срока.
Сбой между DB commit и доставкой не повторяет доменный эффект.
`AI_MODE=legacy` отключает новый runtime без удаления истории семей.

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

Проверено 2026-11-17: запущена сборка `838abe2` с исправлениями независимого ревью;
health: database=ready, telegram=polling, scheduler=ready, ui=buttons,
llm=agent_configured, stt=configured. Cloud session 16348, PID при запуске 49937;
это не systemd service. До запуска старого процесса не было; consumer единственный.
Приватный snapshot перед миграцией:
`/workspace/homequest-private/runtime/before-review-fixes-20261117.db` (0600).
Миграции и запуск на существующей БД успешны, getMe/webhook/models HTTP 200,
webhook отсутствует. Cloud-процесс может остановиться вместе со средой:
проверять health перед использованием. Человеческий voice smoke ещё не выполнен.

Недельный итог планируется по понедельникам в 09:00 семьи в пределах 5 минут,
только если за прошлую неделю были принятые дела. Пропущенные недели не рассылаются.
Цель начинает действовать со следующего понедельника; история политики сохраняется
и для недель без активности. Настройки quiet/mute/vacation применяются к доставке.
Передача дела действует 7 дней; закрытие/переназначение/выход делают её неактуальной.
Покупки не меняют игровой ledger. Новые таблицы находятся в той же SQLite.

## Развёртывание рядом с LiteLLM — подготовлено, не включено

Удалённый сервер пока **не осмотрен и не изменён**. Повтор SSH из Cloud 16 ноября:
TCP/22 ECONNREFUSED до авторизации; managed TCP disabled, VPN не настроен.
Пароль не проверялся на сервере и не передавался в GitHub. Это не доказательство
того, что SSH выключен на самом сервере. Нужен разрешённый SSH/TCP маршрут из
среды (например, настроенный VPN); после его появления продолжать с этого checkout.

1. Сначала выполнить `python3 ops/audit-server.py` на сервере по SSH и сохранить
   JSON вне Git. Скрипт только читает службы, порты, контейнеры, нагрузку, свободные
   память/диск и занятость целевых путей. Не читает env, пароли, командные строки
   процессов или конфиги LiteLLM. Пустой blockers не заменяет инженерную проверку.
2. Определить конкретные LiteLLM, БД и reverse proxy, проверить их health и нагрузку
   до установки; записать baseline без секретов. Проверить Linux amd64/systemd,
   поддержку CPU/memory limits, Python/venv и исходящие Telegram/VyceAI/Hugging Face.
   Начальные консервативные gates: ≥2 CPU, ≥3 GiB MemAvailable, ≥5 GiB свободного
   диска, свободный loopback 2022. Если ресурсов нет — остановить установку и выбрать
   размещение STT отдельно, а не отнимать память у LiteLLM или скрыто отключать голос.
3. После аудита создать отдельные **новые** учётные записи `homequest` (nologin) и
   `homequest-deploy` (только deployment SSH key, без общих admin/docker групп).
   При совпадении существующих пользователей/путей сначала разобраться, не перетирать.
   Корень `/var/lib/homequest-deploy` должен принадлежать root:root 0755; SSH home
   deploy-пользователя — **другой** каталог `/home/homequest-deploy`. Иначе deploy-user
   смог бы подменить родительский каталог root-owned backups.

Целевая раскладка после проверки:

| Путь | Владелец/режим | Назначение |
|---|---|---|
| /opt/homequest, releases | root:root 0755 | неизменяемые проверенные релизы, current symlink |
| /opt/homequest/stt-venv, stt-models | root, runtime читает | отдельный venv и pinned small; системный Python/LiteLLM не меняются |
| /var/lib/homequest | homequest:homequest 0700 | SQLite, lock и временное аудио |
| /var/lib/homequest-deploy | root:root 0755 | родитель, deploy-user не может подменять entries |
| /var/lib/homequest-deploy/incoming | homequest-deploy 0700 | архивы релизов после capacity preflight |
| /var/lib/homequest-deploy/backups | root:root 0700 | последние 7 успешных pre-deploy snapshots |
| /etc/homequest | root:homequest 0750 | конфиг и операторские gates |
| secrets.json, runtime.env | root:homequest 0640 | секреты вне Git; runtime.env из ops/runtime.env.example |
| audit-approved, allow-polling, stt-lock.sha256 | root:root 0644 | аудит, передача consumer, SHA256 requirements-stt.lock |
| /usr/local/sbin/homequest-deploy | root:root 0755 | проверенный ops/deploy.py, не обновляется CI автоматически |
| /etc/systemd/system/homequest.service | root:root 0644 | ops/homequest.service |

Venv устанавливается только в /opt/homequest/stt-venv из requirements-stt.lock
(`pip install --only-binary=:all: -r requirements-stt.lock`). Модель — штатным
scripts/setup-stt.py с STT_MODEL_DIR. Нет Docker/Redis, изменения firewall/nginx,
общего pip install, upgrade ОС или рестарта LiteLLM. Проверить реальный STT и
потребление ресурсов до включения polling. CPUQuota=100% ограничивает HomeQuest
одним ядром, MemoryMax=1536M; это предполагаемые лимиты до измерения на сервере.

После копирования **проверенного** activator на сервер sudoers разрешает только:

```text
homequest-deploy ALL=(root) NOPASSWD: /usr/local/sbin/homequest-deploy *
```

Проверить файл через `visudo -cf`; аргумент activator — один SHA из 40 hex
либо read-only `--check-upload` для проверки ресурсов перед передачей архива.
Deploy-user не получает sudo shell, systemctl, редактор или доступ к secrets.json.
Ключ authorized_keys — с `restrict`; root-пароль не используется в CI. SSH host key
проверяется и закрепляется до передачи ключа в GitHub; StrictHostKeyChecking=yes,
автоматического доверия новому ключу во время CI нет.

### Передача данных и включение автодеплоя

- Создать root-owned audit-approved только после рассмотрения отчёта и baseline.
- Остановить **наш** Cloud consumer, убедиться в завершении, сделать свежий SQLite
  backup и перенести его по SSH в /var/lib/homequest/donetick.db (0600, homequest).
  Перенести секреты защищённым каналом, не в artifact/log; проверить webhook.
- Только после остановки старого consumer создать allow-polling. Первый запуск
  выполнить тем же activator; дождаться database/scheduler ready, telegram polling.
  Повторить health/нагрузку LiteLLM/БД/proxy и сравнить baseline. При ухудшении
  остановить только HomeQuest; выяснить причину, не перезапускать соседние службы.
- В GitHub environment `homequest-production` настроить отдельные secrets:
  HOMEQUEST_DEPLOY_HOST, HOMEQUEST_DEPLOY_KEY, HOMEQUEST_DEPLOY_KNOWN_HOSTS.
  Ограничить deployment branch до codex/homequest-mvp. Включить repository variable
  HOMEQUEST_DEPLOY_ENABLED=true и убрать явный `false` guard job **после** первого
  успешного handoff/проверки LiteLLM.
  До этого deploy job пропускается; GitHub environment/ключи сейчас не созданы.
- CI собирает Linux amd64 без CGO, после всех checks упаковывает только 6 runtime
  файлов и manifest SHA256. `ops/package.py`; artifact хранится 7 дней, без БД/ключей.
  Deploy ждёт checks, сериализуется, пропускает устаревший commit и использует pinned
  SSH host key. На сервере второй deploy блокирует flock. Авто-merge отсутствует.

Активация: ограниченный upload → root-owned копия → whitelist/checksums/commit →
остановка **только homequest.service** → SQLite backup → migration как homequest →
atomic current symlink → start → readiness. При изменении STT lock rollout запрещён
до отдельного обновления venv. Сохраняются 3 последних релиза (плюс current) и 7
snapshots; cleanup только после успешного health. До upload и активации проверяются ≥2 GiB MemAvailable и запас
диска для archive/release/backup; это не заменяет мониторинг размера рабочей БД.

При migration failure новая версия ещё не запускалась: восстанавливается snapshot
и запускается предыдущий код. При startup/readiness failure новая версия могла
принять Telegram updates: сервис останавливается, **БД не откатывается**, snapshot и
candidate сохраняются. Оператор выбирает совместимый код или миграцию вперёд;
обычный restore старого snapshot здесь может потерять данные и требует отдельного
решения. Внешние сервисы и их данные activator не изменяет.

GitHub доступ к коду/PR есть, но текущий CLI integration token получает HTTP 403
Resource not accessible by integration на Actions variables API и Secrets public-key API. Управление
Actions secrets/variables через установленный connector не предоставлено. До
выдачи этого отдельного scope/настройки владельцем CI credentials не установлены.
Обычный repository admin/push статус сам по себе не доказывает эти API permissions.
Поэтому у deploy job есть явный false guard: публикация подготовки ничего не запускает.

## Последний запуск агента — 16 ноября 2026

Cloud session 11906, PID 293683: health проверен после запуска — database/scheduler
ready, telegram polling, ui buttons, llm agent_configured, stt configured.
Предыдущие процессы 119522/148547/207077 — завершённые zombie, новых competing
consumers нет. Использован AI_MODE=agent; команда рестарта приведена в начале.
Backup до миграции: runtime/before-agent-20261116.db, 0600.

agent_configured не означает доступность gateway: первый live smoke Eino прошёл,
затем два tool-calling запроса и минимальный completion получили таймаут 25s.
Не менять ключ/модель на основании таймаута. Повторить opt-in TestLiveEinoRoomPlan
после восстановления ответов VyceAI. Пользовательская проверка: /agent, выбрать
комнату, «Перенеси все дела здесь на завтра в 19:00», проверить и подтвердить план.
Человеческий русский voice проверяется отдельно. Cloud uptime не гарантируется.

## Контроль запуска 18 ноября

В 07:31 UTC после штатной остановки предыдущего consumer сохранён backup
`before-agent-v3-20261118.db` (0600), выполнены миграции, запущен один Cloud-процесс.
В health получены database/scheduler ready, telegram polling, ui buttons,
llm agent_configured, stt configured. Целостность backup и обновлённой БД — ok.
Перезапускать командой из начала файла после остановки собственного consumer.
