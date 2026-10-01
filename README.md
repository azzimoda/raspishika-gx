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
    - `spf13/viper` (конфиг) + `joho/godotenv` (`.env`)
    - API:
        - `gin-gonic/gin`
        - `swaggo/swag` (Swagger-документация)
        - `PuerkitoBio/goquery` (разбор HTML расписания)
    - Бот:
        - `go-telegram/bot`
        - `SevereCloud/vksdk` (VK-бот, Long Poll)
        - `azzimoda/go-tg-proxy` (пул SOCKS5-прокси для Telegram)
        - `gorm.io/gorm` + `gorm.io/driver/sqlite` (SQLite) / `gorm.io/driver/postgres` (PostgreSQL)
        - `pressly/goose/v3` (миграции БД)
        - `chromedp/chromedp`
        - `robfig/cron/v3` (расписания отправки), `rs/zerolog` (логи)
    - Хостинг-утилиты:
        - `avast/retry-go/v5` (повторы отправки и скрейпинга), `github.com/schollz/closestmatch` (подбор похожих названий групп)
- Redis 8.10-alpine (кэш API)
- SQLite 3.53.4 (прод, файл `storage/database/data.db`) / PostgreSQL 16 (dev-стеки и тесты)
- Docker Compose, `chromium` в образе бота

Тесты: `alicebob/miniredis` (Redis), `net/http/httptest` (HTTP-серверы), дымовые проверки с реальными внешними сервисами — по `RT_SMOKE=1`.

## Архитектура

Проект состоит из нескольких сервисов:

- `cmd/bot` — Telegram-бот. Хранит данные в SQLite или PostgreSQL (доступ через GORM, миграции через goose — либо при открытии БД при `DB_AUTO_MIGRATE=true`, либо отдельным сервисом `migrate` в Docker), рендерит скриншоты расписания через собственный браузер Chromium (`chromedp`), расписания получает по HTTP от API.
- `cmd/adminbot` — отдельный админ-бот для мониторинга, статистики и ручных рассылок (`ADMIN_BOT_TOKEN`/`ADMIN_ID`). Работает как отдельный процесс, ручные рассылки отдаёт в очередь `broadcast_jobs`, которую разбирают основные боты.
- `cmd/vkbot` — VK-бот для сообществ: зеркало Telegram-ботa (расписание, преподаватели, настройки беседы) через VK Bots Long Poll (`internal/vkbot/*`, `messenger.VK`). Использует ту же БД и рассылки, отличается колонкой `platform`.
- `cmd/fakevkbot` — VK-бот на фейковых данных (`internal/fakescraper`) без обращения к API, для локальной разработки.
- `cmd/api` — HTTP-сервис скрейпинга. Собирает расписание с `coworking.tyuiu.ru` напрямую по HTTP (`internal/api/scraper`), кэширует результаты в Redis и отдаёт по `/api/v1/*` (Swagger-документация доступна по адресам `/swagger/index.html` (UI) и `/swagger/doc.json`). Бот обращается к нему через `internal/apiclient` по `SCRAPER_HOST`/`SCRAPER_PORT`.
- `cmd/migrate` — одноразовый бинарник: применяет goose-миграции и выходит. В Docker это сервис `migrate` с `restart: "no"`, который остальные боты ждут (см. «Миграции в Docker»).
- `cmd/justray-rotate` — хостовый демон (`internal/justrayrotate`), который проверяет локальный прокси justray и после серии неудач переключает justray на следующий живой не-RU узел. Живёт не в контейнере, а на хосте systemd user-юнитом (см. «justray и ротатор на VPS»).

Для локальной разработки с демо-данными (без реального скрейпинга) есть `cmd/fakeapi` и `cmd/fakebot`.

Образы: `bot.Dockerfile` собирает в один образ `bot`, `adminbot`, `vkbot`, `fakevkbot` и `migrate`
(нужен CGO и Chromium); `api.Dockerfile` — `api` и `fakeapi`. Сервисы `adminbot`, `vkbot` и
`migrate` в compose запускаются из того же образа бота через свой `entrypoint`.

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

Чем стеки отличаются:

| Стек | БД | Образы | API |
| --- | --- | --- | --- |
| `compose.yaml` (прод) | SQLite, файл в `./storage` | готовые `azzimoda/raspishika-{api,bot}:${IMAGE_TAG}` из Docker Hub | реальный скрейпинг |
| `compose.fakeapi.yaml` | PostgreSQL 16 (сервис `db`) | сборка из исходников | `fakeapi` на демо-данных |
| `compose.local.yaml` | PostgreSQL 16 (сервис `db`) | сборка из исходников | реальный скрейпинг |

Во всех стеках есть сервис `migrate` (одноразовый, `restart: "no"`), и `bot`, `adminbot`,
`vkbot` стартуют только после его успешного завершения. В dev-стеках `migrate` дополнительно
ждёт готовности `db`.

Прокси для Telegram в контейнерах указывается как `host.docker.internal:10808`
(`extra_hosts: host-gateway`). В compose подстановка идёт как
`${JUSTRAY_PROXY_ADDR-host.docker.internal:10808}` — с дефисом, а не с двоеточием, поэтому
дефолт подставляется только для **незаданной** переменной. Пустое `JUSTRAY_PROXY_ADDR=` в
`.env` доходит до контейнера как есть и отключает justray: остаётся только бесплатный
список из `PROXY_SOURCE_URL`.

### Make

Ключевые цели Makefile:

```sh
make check        # fmt-check + vet + test + build
make build-bot    # только бот (./cmd/bot, нужен CGO и Chromium)
make build-vkbot  # VK-бот (./cmd/vkbot)
make build-adminbot  # админ-бот (./cmd/adminbot)
make build-api    # API-скрейпер (./cmd/api)
make build-fakeapi / build-fakebot / build-fakevkbot  # демо-бинарники
make build-justray-rotate       # ротатор узлов justray в ~/.local/bin
make install-justray-rotate     # сборка + установка systemd user-юнита
make install-justray-inbound-proxy   # мост docker-сети → 127.0.0.1 justray
make test         # go test ./...
make test-race    # go test -race ./...
make test-pg      # тесты на живом PostgreSQL (TEST_POSTGRES_DSN=...)
make docs         # перегенерировать Swagger-документацию (go generate ./...)
make up           # docker compose up --build -d (реальный API)
make up-fake      # то же, но с демо-данными (compose.fakeapi.yaml)
make up-local     # то же, но API собирается из исходников (compose.local.yaml)
make down         # остановить текущий стек
make logs         # docker compose logs -f
make rollback TAG=<sha>   # откатить прод на предыдущий образ по SHA
make bump-proxy VERSION=<v>   # обновить github.com/azzimoda/go-tg-proxy
make run-api / run-vkbot / run-fakeapi / run-fakebot / run-adminbot / run-fakevkbot
make clean        # go clean + docker compose down
```

Цели `up-*`/`down`/`logs` работают с одним стеком и снимают ровно его: `down`
поднимает `STACK` (по умолчанию `fakeapi`), а `down-fake-ro`/`down-local-ro`
разбирают оба dev-стека — так демо-стек не сносит прод-контейнеры, и наоборот.

Полный список — `make help`.

### Тесты

```sh
make test         # весь стек без внешних сервисов: Redis — miniredis, HTTP — httptest
make test-race    # то же под -race (клиент VK, ротатор и сервисный слой с горутинами)
make test-pg      # диалектные тесты на живом PostgreSQL; чистят за собой строки
RT_SMOKE=1 go test ./internal/justrayrotate/ ./internal/service/   # реальные justray/Telegram-пробы
```

`make test-pg` читает `TEST_POSTGRES_DSN` (по умолчанию — локальный PostgreSQL на
`127.0.0.1:5432`, пользователь и база `raspishika`) и гоняет только тесты с
`-run Postgres` в `pkg/database` и `internal/repository`. Пакеты сериализованы
(`-p 1`): оба поднимают миграции в одной базе, и параллельный запуск раньше падал на
`relation "goose_db_version" does not exist`. Без `TEST_POSTGRES_DSN` эти тесты
пропускаются, так что обычному `make test` PostgreSQL не требуется.

### Миграции в Docker

Сервис `migrate` (`cmd/migrate`) применяет goose-миграции один раз и выходит.
Бот, админ-бот и VK-бот запускаются с `DB_AUTO_MIGRATE=false` и ждут его
завершения (`service_completed_successfully`), поэтому четыре процесса не
гоняют `goose.Up` по одному SQLite-файлу одновременно. Локально (вне Docker)
миграции по-прежнему применяются при открытии БД — за это отвечает
`DB_AUTO_MIGRATE` (по умолчанию `true`).

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

Workflow `.github/workflows/deploy.yaml` срабатывает на push в `main`, но только если
изменились `cmd/**`, `internal/**`, `pkg/**`, `migrations/*`, `templates/*`, `compose.yaml`,
`configs/**`, `Makefile`, `*.Dockerfile`, `go.*` или сам workflow — правка README или
`.env.example` деплой не гоняет.

Затем два джоба:

- **verify** — `gofmt -l internal/ cmd/ pkg/`, `go vet ./...`, `go test ./...`,
  `go build ./...` (те же шаги, что делает `make check`; упавший gate останавливает деплой);
- **deploy** — сборка `api.Dockerfile` и `bot.Dockerfile` с публикацией в Docker Hub
  дважды: тегом `latest` и тегом commit SHA. Старые образы намеренно не вычищаются —
  без них откат невозможен.

Дальше deploy по SSH на VPS: `git pull`, точечная замена в `.env` строки `IMAGE_TAG` на
SHA коммита (секреты в `.env` не трогаются), затем `docker compose pull && down && up -d`
и `docker compose ps`. Миграции применяет сервис `migrate` (см. выше). Успех и провал
workflow уходят сообщением в Telegram через `ADMIN_BOT_TOKEN`/`ADMIN_ID`.

### Откат

Образы тегируются по SHA, поэтому откат — это правка одной строки и повторный запуск стека:

```sh
make rollback TAG=<sha>   # правит IMAGE_TAG в .env и поднимает стек на этом образе
docker compose ps         # убедиться, что migrate завершился, а сервисы healthy
```

### justray и ротатор на VPS

Прокси для Telegram живёт на хосте, а не в контейнере: контейнеры ходят в
`host.docker.internal:10808` (`extra_hosts: host-gateway` в compose).

1. **justray в режиме прокси.** Включить mixed in-bound на `127.0.0.1:10808`
   в режиме прокси, **оставив `allow_lan` выключенным**: в justray это булев
   переключатель без адреса, поэтому `on` слушает `0.0.0.0` и публичный SOCKS5
   оказывается на IP VPS. Проверить: `ss -ltnp | grep 10808` должен показать
   только `127.0.0.1:10808`.
2. **Мост docker → justray.** Контейнеры до `127.0.0.1` не достучатся, а
   `allow_lan` включать нельзя, поэтому docker-мост пробрасывается на loopback
   in-bound: `make install-justray-inbound-proxy` (снять —
   `make uninstall-justray-inbound-proxy`). Ставится пара
   `justray-inbound-proxy.socket` + `.service`: сокет слушает **только** адрес
   docker-моста (то, во что резолвится `host.docker.internal` в контейнерах), а
   `systemd-socket-proxyd` перекладывает байты в `127.0.0.1:10808`. Байты
   идут как есть, поэтому смешанному in-bound не важно, SOCKS5 там был или HTTP.
   На публичном интерфейсе не слушает ничего, правило файрвола на 10808 не
   нужно. Адрес и наличие `systemd-socket-proxyd` берутся из живой системы, а
   при неудаче установка падает, а не подставляет `0.0.0.0`.
3. **Ротатор.** `make build-justray-rotate` и `make install-justray-rotate`
   ставят `justray-rotate.service` как systemd **user**-юнит
   (`configs/justray-rotate.service`); `make uninstall-justray-rotate` его
   снимает. Он дергает `JUSTRAY_PROBE_URL` через `JUSTRAY_PROBE_PROXY` и после
   `JUSTRAY_FAILURE_THRESHOLD` неудач переключает justray на следующий живой
   не-RU узел по кругу (`justray subscription list --json`, `JUSTRAY_EXCLUDE`
   добавляет свои подстроки к исключениям по умолчанию). `JUSTRAY_PROBE_PROXY`
   — адрес с самого хоста (по умолчанию `127.0.0.1:10808`), и он намеренно не
   равен `JUSTRAY_PROXY_ADDR`: ротатор крутится на хосте, где
   `host.docker.internal` не резолвится, и проба падала бы всегда. Проверка
   вживую: `systemctl --user status justray-rotate`, журнал —
   `journalctl --user -u justray-rotate`.
4. **Переменные.** В `.env` на VPS: `JUSTRAY_PROXY_ADDR` (для контейнеров
   `host.docker.internal:10808`), секреты VK (`VK_GROUP_TOKEN`, `VK_GROUP_ID`) и
   `IMAGE_TAG` последнего рабочего SHA.
5. **Проверка, что боты реально пошли через justray.**

   ```sh
   docker compose up -d
   docker compose logs bot vkbot adminbot | grep -i "using proxy"
   ```

   Ожидается в каждом процессе строка с `proxy=host.docker.internal:10808`:
   `Telegram bot using proxy` — основной бот, `Admin reporter using proxy` —
   админ-бот (он поднимается во всех трёх: `bot`, `vkbot` и отдельный
   `adminbot`). В процессе `vkbot` это единственная такая строка, и это
   нормально: сам VK-клиент через прокси не ходит, пул использует только
   админ-репортер внутри него.

   **Не считать проблемой `WRN Pool proxy dropped error="proxy unavailable"`.**
   `go-tg-proxy` при каждой ревалидации тёплого пула проверяет его целиком и
   пишет такую строку на каждый мёртвый бесплатный прокси, а бан-лист
   (`internal/proxyfail`) живёт в памяти, поэтому после рестарта контейнера
   проверяется весь список заново. Пока justray жив, бесплатные прокси
   не используются, но продолжают опрашиваться: эти предупреждения — фон, а не
   признак поломки justray. Признак настоящей проблемы — в строках `using proxy`
   вместо justray стоит чужой адрес.

---

## Ссылки

"Официальный" бот, поддерживаемый мной: [@RaspishikaBot](https://RaspishikaBot.t.me)

Мой телеграм-канал: [@mazzaLLM](https://mazzaLLM.t.me)

Поддержи меня:

- [DonationAlerts](https://www.donationalerts.com/r/azzimoda)
- [YooMoney](https://yoomoney.ru/to/4100119212250883)
- Gram: `UQCFh_yK4yLHwfRWrn-inUNYqw5boabRLmDtm5SEZf8SbDO1`
- Звёздами в [ТГК](https://mazzaLLM.t.me)
