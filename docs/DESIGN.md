# Карточки HomeQuest

Нативная тема Telegram, Open Props 1.7.23 (size-2/3, radius-3), Lucide Static 1.73.0
(house). LICENSE в docs/licenses. build-widgets.mjs извлекает токены при сборке;
в сообщении нет CDN, React, внешних шрифтов, CSS imports или запросов в сеть.

`widgets/family.html` — единый продуктовый шаблон: заголовок, текст, список действий.
Панель использует --tg-theme-bg-color/text-color; шрифт системный; кнопки ≥44px.
Пользовательские строки вставляются через textContent. Фото приходят отдельным
Telegram media message. Public_data не используется, состояние — проекция для viewer.

Ограничения проверяются в Go и сборке: HTML 32768, zlib9 4096 (цель 3500), state
1024, request 64 байта. Пагинация заданий/подарков/наград — одна позиция на карточку.
В списках длинные названия сокращаются; подробности подарка доступны отдельно.

Официальный QuickJS + validator + renderer: E02 на 200/320/390px, light/dark/custom;
дополнительно 17 продуктовых карточек на 200px, высота ≤600px. Скриншоты из того же
renderer: [light](screenshots/home-light.png), [dark](screenshots/home-dark.png),
[custom](screenshots/home-custom.png). Проверка выявила и исправила прозрачный фон
панели на белом тестовом canvas тёмной темы. Это не скриншоты Telegram-клиента.

Пока используется одна Lucide house, а не весь задуманный набор иконок.
Полная визуальная приёмка каждого редкого состояния и живого клиента остаётся открытой.
