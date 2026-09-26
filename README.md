# Бот Распиши-ка (Telegram + VK)

Бот для удобного доступа к расписанию студентов и преподавателей МПК ТИУ. Работает в двух мессенджерах: Telegram (`cmd/bot`) и VK (`cmd/vkbot`), используя общую базу данных и очереди рассылок.

> [!important]
> Этот бот **не имеет прямого отношения к Многопрофильному колледжу** и является **моим личным проектом**. По всем вопросам следует обращатся [ко мне лично](#ссылки).
>
> Официальный источник расписания: https://coworking.tyuiu.ru/shs/index.php

---

## Функционал бота

Для быстрого получения расписания своей группы и для рассылок необходимо задать свою группу. Бот предложит задать её сразу после начала чата (/start), а также её можно задать в меню настроек (/settings).

Не задавая группу можно быстро получить расписание на неделю любой группы просто прислав её название, не обязательно в строгом формате. Например: в ответ на сообщение "испт 22 9 2" бот отравит расписание на неделю этой группы ИСПт-22-(9)-2.

### Форматы расписания

Для получения расписание своей группы доступны команды `/today` (`сегоня`), `/tomorrow` (`завтра`) и `/week` (`неделя`).

Для получения расписания преподавателя необходимо использовать комманду `/teacher`, далее написать полное имя преподавателя или его часть, затем выбрать нужного преподавателя из предложенного списка совпадений. Также в отправляемом сообщении в ответ на команду `/teacher` будут появляться 4 последних преподавателя, для которых была использована команда.

### Рассылки

Доступны 3 вида рассылок:

1. ежедневная рассылка — каждый день в заданное время бот присылает расписание заданной группы на неделю;
2. напоминание перед началом пар — за 15 минут до начала каждой пары бот присылает текстовое напоминание с информацией о ней;
3. уведомление об изменениях в расписании — когда включено, бот регулярно проверяет расписание заданной группы на наличие изменений в нём и при обнаружении отправляет текстовое сообщение с описанием измемений.

### Групповые чаты

В групповых чатах есть возможность настроить доступ обычных пользователей (не админов) к командам бота.

### Бот администратора

Есть возможноть привязать дополнительного бота для мониторинга состояния основного бота и статистики. Для этого необходимо указать токен администратора в переменной окружения `ADMIN_BOT_TOKEN` и ID администратора в `ADMIN_ID`.

### VK-бот (`cmd/vkbot`)

Зеркало Telegram-бота для сообществ VK через VK Bots Long Poll: то же расписание (группы и преподаватели) с навигацией по дням, настройки беседы и пейджинг. VK-бот **не предоставляет админ-функций** (доступ, `access`/`setaccess`) — они есть только в Telegram.

Для запуска нужны сообщество VK с включённым Long Poll и ключ доступа: `VK_GROUP_TOKEN`, `VK_GROUP_ID`, `VK_API_VERSION` (по умолчанию `5.199`). Ключи задаются в `.env` — см. [Сборка и запуск](#сборка-и-запуск).

Рассылки (ежедневная, перед парами, об изменениях) работают и для VK: чаты платформы различаются колонкой `platform`, задачи `broadcast_jobs` обслуживаются своим процессом. Для отладки без живого скрейпинга есть `cmd/fakevkbot` с фейковыми данными (`internal/fakescraper`).

---

## Стек

Основные использованные технологии:

- Go v1.26
    - `spf13/viper`
    - API:
        - `gin-gonic/gin`
        - `swaggo/swag` (Swagger-документация)
    - Бот:
        - `go-telegram/bot`
        - `SevereCloud/vksdk` (VK-бот, Long Poll)
        - `gorm.io/gorm` + `gorm.io/driver/sqlite` (SQLite) / `gorm.io/driver/postgres` (PostgreSQL)
        - `pressly/goose/v3` (миграции БД)
        - `chromedp/chromedp`
- Redis (кэш API)
- SQLite v3.37

## Архитектура

Проект состоит из нескольких сервисов:

- `cmd/bot` — Telegram-бот. Хранит данные в SQLite или PostgreSQL (доступ через GORM, миграции применяются автоматически при запуске через goose), рендерит скриншоты расписания через собственный браузер Chromium (`chromedp`), расписания получает по HTTP от API.
- `cmd/adminbot` — отдельный админ-бот для мониторинга, статистики и ручных рассылок (`ADMIN_BOT_TOKEN`/`ADMIN_ID`). Работает как отдельный процесс, ручные рассылки отдаёт в очередь `broadcast_jobs`, которую разбирают основные боты.
- `cmd/vkbot` — VK-бот для сообществ: зеркало Telegram-ботa (расписание, преподаватели, настройки беседы) через VK Bots Long Poll (`internal/vkbot/*`, `messenger.VK`). Использует ту же БД и рассылки, отличается колонкой `platform`.
- `cmd/fakevkbot` — VK-бот на фейковых данных (`internal/fakescraper`) без обращения к API, для локальной разработки.
- `cmd/api` — HTTP-сервис скрейпинга. Собирает расписание с `coworking.tyuiu.ru` напрямую по HTTP (`internal/api/scraper`), кэширует результаты в Redis и отдаёт по `/api/v1/*` (Swagger-документация доступна по адресам `/swagger/index.html` (UI) и `/swagger/doc.json`). Бот обращается к нему через `internal/apiclient` по `SCRAPER_HOST`/`SCRAPER_PORT`.

Для локальной разработки с демо-данными (без реального скрейпинга) есть `cmd/fakeapi` и `cmd/fakebot`.

## Сборка и запуск

### Docker

Поднять Redis, API и ботов (Telegram + VK):

```sh
docker compose up --build
```

Для локальной разработки с демо-данными (вместо реального скрейпинга):

```sh
docker compose -f compose.fakeapi.yaml up --build
```

Для локальной разработки с API из исходников (реальный скрейпинг):

```sh
docker compose -f compose.local.yaml up --build
```

VK-бот (`vkbot`) поднимется в любом из этих стеков, если заданы `VK_GROUP_TOKEN` и `VK_GROUP_ID`; без них соответствующий контейнер будет падать с ошибкой. Демо-редакция `cmd/fakevkbot` (без обращения к API) в compose не входит и запускается локально: `go run ./cmd/fakevkbot`.

### Make

Ключевые цели Makefile:

```sh
make check        # fmt-check + vet + test + build
make build-bot    # только бот (./cmd/bot, нужен CGO и Chromium)
make build-vkbot  # VK-бот (./cmd/vkbot)
make build-api    # API-скрейпер (./cmd/api)
make test         # go test ./...
make docs         # перегенерировать Swagger-документацию (go generate ./...)
make up           # docker compose up --build -d (реальный API)
make up-fake      # то же, но с демо-данными (compose.fakeapi.yaml)
make up-local     # то же, но API собирается из исходников (compose.local.yaml)
make logs         # docker compose logs -f
```

Полный список — `make help`.

### Ручная сборка

1. Клонировать репозиторий:
   
   ```bash
   git clone https://github.com/azzimoda/raspishika-gx.git
   ```

2. Установить зависимости:
   
   ```bash
   go mod download
   ```

3. Собрать API-сервис:
   
   ```bash
   go build ./cmd/api
   ```

4. Собрать бота (нужен CGO для sqlite3 и Chromium в PATH для скриншотов):
   
   ```bash
   go build ./cmd/bot
   ```

5. (Опционально) Собрать VK-бота:
   
   ```bash
   go build ./cmd/vkbot
   ```

6. Запустить Redis (для кэша API):
   
   ```bash
   docker run --rm -p 6379:6379 redis:alpine
   ```

7. Подготовить конфигурацию: скопировать `.env.example` в `.env` и указать свои значения (минимум — токен бота). Все ключи с комментариями перечислены в `.env.example`:

   ```bash
   cp .env.example .env
   ```

   Например, все необходимые ключи выглядят так:

   ```bash
   # Required
   BOT_TOKEN=your_bot_token_here
   
   # VK-бот (нужно только если запускается cmd/vkbot)
   VK_GROUP_TOKEN=your_vk_group_token_here
   VK_GROUP_ID=your_vk_group_id_here

   # Optional
   ADMIN_BOT_TOKEN=your_admin_bot_token_here
   ADMIN_ID=admin_user_id_here
   REDIS_PASSWORD=your_redis_password_here
   ```

8. Запустить API-сервис и бота:
   
   ```bash
   ./api
   ./bot
   ```

Альтернативно, вместо реального API можно запустить `cmd/fakeapi` с демо-данными (`go build ./cmd/fakeapi && ./fakeapi`), или `cmd/fakebot` без API сервиса (`go build ./cmd/fakebot && ./fakebot`). VK-бот запускается аналогично: `go build ./cmd/vkbot && ./vkbot` (нужны `VK_GROUP_TOKEN`/`VK_GROUP_ID`), а `cmd/fakevkbot` — с фейковыми данными и без API.

## Развёртывание

При push в ветку `main` GitHub Actions (`.github/workflows/deploy.yaml`) собирает и публикует образы `raspishika-api` и `raspishika-bot` в Docker Hub, после чего по SSH перезапускает сервисы на VPS (`docker compose pull && docker compose up -d`).

---

## Роадмап

План интеграции VK-версии бота (`raspishika-vk`) в этот репозиторий.

### Уже сделано

- **Фаза 1** — платформенный дискриминатор `platform` в таблице `chats` (Telegram/VK могут иметь одинаковые peer id), рантайм на PostgreSQL и миграции в `migrations/postgres/`.
- **Фаза 2a** — мессенджер-нейтральный слой `internal/messenger` (Telegram-адаптер) и общий `BroadcastService`; рассылки больше не привязаны к конкретной платформе.
- **Фаза 2b** — сервис БД в Docker-стеке, полная совместимость SQLite/PostgreSQL (миграционный тест на живом PostgreSQL).
- **Фаза 2c** — ручные рассылки через очередь `broadcast_jobs` (по одной задаче на платформу, воркер в каждом процессе забирает только свои), админ-бот вынесен в отдельный бинарник `cmd/adminbot` (report-only, без браузера). Прод остаётся на SQLite.
- **Фаза 2d** — VK-бот на `SevereCloud/vksdk`: обёртка клиента `internal/vkbot/client` (Long Poll с reconnect и дедупом, отправка сообщений/фото, проверка прав админа сообщества), обработчики `internal/vkbot/main` (зеркало TG-хендлеров без админ-функций: расписание с навигацией по дням, поиск преподавателей, настройки беседы и пейджинг), разметка клавиатур в `internal/vkbot/util`, адаптер `messenger.VK`. Отдельный бинарник `cmd/vkbot`, демо `cmd/fakevkbot` на фейковых данных (`internal/fakescraper`). Рассылки покрывают `platform=vk`. Ручной smoke на реальном Long Poll сообщества пройден. Конфиг: `VK_GROUP_TOKEN`, `VK_GROUP_ID`, `VK_API_VERSION`.
- **Фаза 3 (частично)** — единая сборка образа: `bot.Dockerfile` собирает `bot`, `adminbot`, `vkbot` и `fakevkbot`; сервис `vkbot` добавлен во все compose-файлы (`compose.yaml`, `compose.fakeapi.yaml`, `compose.local.yaml`, PostgreSQL в dev-стеках). Демо-бинарник `fakevkbot` в compose не разворачивается и доступен локально.

### Впереди

- Принятие решения о переезде прода с SQLite на PostgreSQL (на текущий момент прод работает на SQLite, и с VK-ботом несколько процессов делят один SQLite-файл).

**Фаза 4 — финализация:**

- Smoke-проверка всего стека (Telegram-бот + VK-бот + админ-бот) в Docker, исправления по итогам.
- Обновление SCRAPER под готовую архитектуру (при необходимости).

---

## Ссылки

"Официальный" бот, поддерживаемый мной: [@RaspishikaBot](https://RaspishikaBot.t.me)

Мой телеграм-канал: [@mazzaLLM](https://mazzaLLM.t.me)

Поддержи меня:

- [DonationAlerts](https://www.donationalerts.com/r/azzimoda)
- [YooMoney](https://yoomoney.ru/to/4100119212250883)
- Gram: `UQCFh_yK4yLHwfRWrn-inUNYqw5boabRLmDtm5SEZf8SbDO1`
- Звёздами в [ТГК](https://mazzaLLM.t.me)
