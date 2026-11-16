# Карточки HomeQuest

Основа: нативная тема Telegram, Open Props 1.7.23 (size-2/3, radius-3), Lucide Static
1.73.0 (house). LICENSE в docs/licenses. Сборщик scripts/build-widgets.mjs читает
CSS exports на этапе build; в сообщении нет CDN, CSS imports, внешней сети или React.

widgets/home.html — compatibility probe, не итоговый экран семьи.
Фон панели --tg-theme-secondary-bg-color, корень прозрачный, системный шрифт.
Кнопка минимум 44px; состояния обозначены текстом; обновление через textContent.
Изменяемые данные только chat_data; public_data отсутствует.
Бюджеты: 32768 HTML, 4096 zlib9 (целевой 3500), 1024 state, 64 request байта.
Текущая карточка: 1392 HTML / 813 zlib9. Официальный runtime проверен на 200/320/390px,
light/dark/custom. Скриншоты и остальные продуктовые карточки пока отсутствуют.
