# Изолированный Eino/VyceAI spike

Проверка из исследования docs/AGENT_RESEARCH.md. Не подключена к production bot.
Реальный Eino v0.9.23 + OpenAI adapter v0.1.13, реальный разрешённый gateway;
инструменты и комната синтетические, БД HomeQuest не открывается.

```sh
cd experiments/agent-spike
GOTOOLCHAIN=local go build -o /tmp/homequest-agent-probe .
HOMEQUEST_SECRETS_FILE=/absolute/private/secrets.json /tmp/homequest-agent-probe -live
```

Go 1.24.3. Без -live сеть не используется. Читает существующие NICKLLM_* из среды
или приватного JSON; ключи не печатает, redirects запрещены. Привязан к уже
проверенным VyceAI URL/model ID, без выбора другой модели. Ошибки обезличены.
Live-запуск расходует небольшое число запросов; в CI автоматически не запускается.

PASS 16.11.2026: 1 get_room_tasks, 1 prepare_reschedule, правильные ID/дата;
committed_changes=0. Не доказывает STT, реальные доменные действия, MCP,
авторизацию, сохранение сессии или interrupt/resume после restart.
