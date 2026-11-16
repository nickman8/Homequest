# Статус — 2026-11-16

Пробная версия развивается; полный MVP ещё не завершён.

Рабочие вертикальные сценарии: дом/приглашение/участники; задания, назначение,
редактирование, срок, отмена/пауза; независимые фото/approval, undo 5 минут;
календарные и интервальные повторы; напоминания, тихие часы, snooze, отпуск;
баланс/вклад недели/XP, рейтинг, 6 достижений; каталог наград с выдачей/refund;
wishlist с секретной атомарной бронью; ручной семейный Premium без платежей.

Все сущности в одной SQLite базе Donetick. Баллы в upstream PointsHistory,
цена фиксируется в попытке/заявке; расходы не меняют XP/вклад. При смене timezone
повторы сохраняют местные часы, разовые UTC-сроки и история не перемещаются.
Старые карточки и reminder jobs инвалидируются версией экземпляра.

Проверено: go test ./...; продуктовые bot flows с контролируемым transport;
официальный QuickJS/renderer, E02 на 200/320/390px и light/dark/custom (18 checks
включая compatibility spike). Это не live-проверка Telegram-клиента.

Осталось: конкретный проверяющий, более полные E04–E13 связи через bot handlers,
полировка диалогов/длинных карточек, AI/STT, расширение E02 на вклад/XP,
финальные screenshots и проверка всех экранов. Сохранение достижения происходит
при открытии раздела; это ограничение текущей реализации.

Telegram getMe: @Home_questbot, webhook отсутствует. Повторный NickLLM /models:
HTTP 403. ID DeepSeek 4.1 Flash не подтверждён, completion не выполнялся.
Русский STT smoke и живое голосовое ещё не пройдены.

Временный polling ранее запущен со старым binary; новые функции требуют сборки
и контролируемого рестарта (RUNBOOK). DATA_DIR вне Git:
/workspace/homequest-private/runtime. Health: http://127.0.0.1:2022/health.
Срок жизни Cloud-процесса не гарантируется. Не публиковать БД/секреты в Git.

Git: ветка codex/homequest-mvp опубликована через Git Data API. Существующая
интеграция авторизует API, но не HTTPS Git. Создание PAT через неё недоступно;
новый токен не потребовался. main/master не создавались; отдельной базы для PR нет.
Remote snapshot имеет другой commit SHA при идентичном tree; source SHA в message.

Продолжить: make test GO=/workspace/toolchains/go/bin/go; make harness
GO=/workspace/toolchains/go/bin/go WIDGET_SANDBOX=/workspace/widget-sandbox;
затем закрыть перечисленные ограничения, обновить процесс, STATUS и TEST_MATRIX.
