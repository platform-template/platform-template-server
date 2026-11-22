# Audit service

## Audit: домен и хранилище

`AuditEntry` — неизменяемая запись о действии. `EventID` хранится строкой: SDK обычно генерирует UUIDv7, но контракт envelope допускает и другие непустые строковые идентификаторы. Максимальная длина — 255 символов; более длинные идентификаторы пока не поддерживаются.

Репозиторий поддерживает добавление, получение по EventID и поиск по исполнителю, модулю, действию, объекту и времени. Записи сортируются по времени события и EventID по убыванию. Для времени используется интервал `[From, To)`, limit по умолчанию — 20, максимум — 100. Исполнитель может отсутствовать, UUID исполнителя хранится в `BINARY(16)`; время нормализуется к UTC с точностью до микросекунд.

Metadata содержит валидный JSON размером до 16 КиБ; при отсутствии используется `{}`. Будущие обработчики должны передавать только явно разрешённые поля: пароли, токены, cookies, секреты, полные HTTP-тела и ненужные персональные данные сохранять нельзя. Автоматическая очистка Metadata пока не реализована.

Репозиторий не предоставляет изменение или удаление записей; это ограничение приложения, а не защита от прямого SQL пользователя с соответствующими правами. Дедупликация EventID обеспечивается первичным ключом MySQL.

## Чтение журнала

`GET /api/public/v1/audit/entries` возвращает записи с фильтрами `actorId`, `moduleKey`, `action`, `targetType`, `targetId`, `from`, `to`, `limit` и `offset`. `GET /api/public/v1/audit/entries/by-event-id?eventId=...` возвращает одну запись. Через Gateway пути начинаются с `/audit/`.

Audit передаёт cookie `session` в Authz `IsSuperuser`; Authz проверяет её через Session. Чтение доступно только Superuser с глобальными привилегиями. Назначение доступа к модулю `audit` само по себе пока не даёт доступа к журналу: отдельная политика чтения журнала не определена. При недоступности Authz или Session доступ закрыт. Проверка действует и при прямом запросе к Audit. Внутренние gRPC-соединения платформы должны оставаться в доверенной сети: межсервисная аутентификация и шифрование пока не настроены.

Рабочий пример использования go-sdk и исходник для генератора новых модулей.

## Сценарий

```text
HTTP / gRPC → app → unit of work → echo + outbox (одна MySQL-транзакция)
                                             ↓
                                      relay → RabbitMQ
                                             ↓
                              message-handler → inbox → лог
```

HTTP: `POST /api/public/v1/echo` с JSON `{"body":"hello"}`. Ответ:
`{"operationID":"<uuid>"}`. Пустая строка допустима; поле `body` обязательно.

gRPC: `auditInternalAPI.AuditService/Echo`, запрос `{"Body":"hello"}`,
ответ с `OperationID`. Сервер поддерживает reflection:

```sh
grpcurl -plaintext -d '{"Body":"hello"}' localhost:8081 \
  auditInternalAPI.AuditService/Echo
```

Событие `auditEvents.Echo` содержит ID echo и исходную строку. SDK добавляет
envelope с event ID, producer, type, временем и correlation ID. Общий exchange
`domain-event` находится в `lib/event`; routing key модуля — `audit.Echo`.

## Структура

- `cmd`: конфигурация, команды запуска и файлы сборки зависимостей по подсистемам: MySQL, миграции, AMQP, события и серверы.
- `internal/audit/app`: транзакционная граница через SDK `UnitOfWorkWithRepositoryProvider`.
- `internal/audit/domain`: echo-модель, бизнес-операция, интерфейс репозитория и payload события; используется нативный пакет `uuid` Go 1.27.
- `internal/audit/infra/mysql`: repository provider; `repository` содержит реализацию echo-репозитория, `migrations` — миграции модуля.
- `internal/audit/infra/handlers`: HTTP, gRPC и события.
- `internal/integration-tests`: инфраструктура, контейнеры и интеграционные проверки.
- `api`: protobuf/OpenAPI-контракты; generated Go-код исключён из Git.
- `config/gateway`: маршруты и объявления интерцепторов модуля для общего gateway.

Repository provider хранит транзакционный клиент и выдаёт репозиторий с контекстом
операции. `app` использует `ExecuteWithRepositoryProvider`; SQL-клиент в него не
передаётся. Outbox и репозиторий используют один context и один экземпляр UOW.

UUID в таблице `echo` хранится как `BINARY(16)`. В SQL-параметры передаётся
`uuid.UUID`: MySQL-клиент SDK преобразует его в 16 байт. В HTTP, gRPC и событиях
ID остаётся строкой.

## Пример интерцептора gateway

`internal/audit/infra/handlers/gateway` реализует gRPC API интерцептора через
`lib/gateway/interceptor` и регистрируется на сервере
модуля (`8081`) вместе с internal API. `config/gateway/interceptor.yaml` объявляет
интерцептор, а `routes.yaml` подключает его к маршруту `/audit/api/...`.
`local-up` автоматически собирает оба файла в конфигурацию gateway.

Пример показывает три решения:

- `requestHeaders`: устанавливает `X-Demo-Request-ID` из `context.requestId`, заменяя значение клиента.
- `responseHeaders`: добавляет `X-Demo-Interceptor: audit` в ответ backend.
- `requestHeaders` с `X-Demo-Deny: true`: возвращает JSON с HTTP `403` через `immediateResponse`; backend и response-фаза не вызываются.

Это демонстрация API. Проверку заголовка `X-Demo-Deny` следует заменить политикой
приложения, если нужна авторизация или другая бизнес-проверка.

После `mise run local-up`:

```sh
curl -i -X POST localhost/audit/api/public/v1/echo \
  -H 'Content-Type: application/json' -d '{"body":"through gateway"}'
curl -i -X POST localhost/audit/api/public/v1/echo \
  -H 'Content-Type: application/json' -H 'X-Demo-Deny: true' -d '{"body":"denied"}'
```

Первый ответ содержит `X-Demo-Interceptor: audit`, второй — `403`.
Для отключения примера удалите `interceptors` у маршрута. При обращении к HTTP
порту модуля напрямую интерцептор не вызывается.

## Запуск

Из корня репозитория:

```sh
mise run local-up
```

Корневой mise автоматически подключает модульный `compose.yaml`, собирает бинарник
и поднимает сервис и consumer в контейнерах.

Конфиги из `config/gateway/` автоматически собираются вместе с конфигами
остальных модулей при `local-up`. Общие настройки gateway находятся в корневом
`config/gateway/`. Маршрут `/audit/api/...` проксируется на
`audit-service:8082`, удаляя префикс `/audit`:

```sh
curl -X POST localhost/audit/api/public/v1/echo \
  -H 'Content-Type: application/json' -d '{"body":"hello"}'
```

После изменения конфигов повторите `mise run local-up` для перезапуска gateway.

Бинарник поддерживает `service` (по умолчанию) и `message-handler`.
`service` запускает HTTP, gRPC и relay. `message-handler` запускает consumer
и HTTP `/healthz`. Health endpoint отвечает 204 и проверяет liveness.

## Конфигурация

| Переменная | Значение по умолчанию |
| --- | --- |
| `AUDIT_APP_DEBUG` | `false` |
| `AUDIT_APP_GRACE_TIMEOUT` | `30s` |
| `AUDIT_HTTP_ADDRESS` | `:8082` |
| `AUDIT_GRPC_ADDRESS` | `:8081` |
| `AUDIT_AUTHZ_ADDRESS` | `authz-service:8081` |
| `AUDIT_MYSQL_ADDRESS` | `localhost:3306` |
| `AUDIT_MYSQL_DATABASE` | `audit` |
| `AUDIT_MYSQL_USER` | `audit` |
| `AUDIT_MYSQL_PASSWORD` | `audit` |
| `AUDIT_MYSQL_MAX_CONNECTIONS` | `10` |
| `AUDIT_MYSQL_CONNECTION_MAX_LIFETIME` | `30m` |
| `AUDIT_MYSQL_CONNECTION_MAX_IDLE_TIME` | `5m` |
| `AUDIT_MYSQL_CONNECT_TIMEOUT` | `5s` |
| `AUDIT_MYSQL_READ_TIMEOUT` | `10s` |
| `AUDIT_MYSQL_WRITE_TIMEOUT` | `10s` |
| `AUDIT_AMQP_URL` | `amqp://local:local@localhost:5672/` |
| `AUDIT_AMQP_QUEUE` | `audit.echo` |
| `AUDIT_AMQP_CONNECT_TIMEOUT` | `5s` |
| `AUDIT_AMQP_PREFETCH_COUNT` | `10` |
| `AUDIT_RELAY_INTERVAL` | `1s` |
| `AUDIT_RELAY_BATCH_SIZE` | `100` |
| `AUDIT_RELAY_PUBLISH_TIMEOUT` | `10s` |
| `AUDIT_RELAY_LOCK_TIMEOUT` | `1s` |

DSN формируется рядом с connector через `mysqldriver.Config.FormatDSN()`;
`parseTime` и `utf8mb4` включаются там же. `--debug` также включает подробные логи.
Новые модули используют свой env-префикс.

## Миграции и сборка

```sh
mise run //modules/audit:generate-migration "Create a new table"
mise run //modules/audit:build
```

Генератор создаёт и регистрирует новую миграцию. Реализуйте `Up`: начальная
заглушка возвращает ошибку. Обе команды сервиса применяют все таргеты миграций:
модуль, outbox и inbox.

`build` генерирует контракты и локально собирает Linux-бинарник `dist/audit`.
Dockerfile только копирует его в образ. Для нативного запуска используйте задачу
`run`, для выбора архитектуры Linux-сборки — `GOARCH`.

## Гарантии

Успех API означает commit echo и outbox. Ошибка внутри транзакции откатывает обе
записи. Повторный API-запрос создаёт новый echo.

Оба процесса объявляют durable-очередь. Relay использует publisher confirms и
повторяет неудавшиеся публикации. API сохраняет операции при потере RabbitMQ
после запуска; backlog отправляется после восстановления соединения. Ошибка
подключения при первоначальном запуске останавливает процесс.

Доставка at-least-once. Inbox подавляет повторную обработку зафиксированных
`(consumer, event_id)`. Логирование не атомарно с commit inbox: при сбое возможен
повторный лог. Ошибки consumer и невалидные события попадают в `<queue>.dead`
через общий exchange `domain-event.dead`. Автоматический retry consumer не включён.

HTTP и gRPC принудительно закрываются после grace timeout и возвращают ошибку,
для которой `errors.Is(err, context.DeadlineExceeded)` истинно.
Очистка outbox/inbox через SDK `Cleanup` в пример не входит.

## Проверки

```sh
mise run //modules/audit:all
mise run //modules/audit:integration-tests
```

Интеграционные тесты проверяют HTTP и gRPC API, запись события в outbox,
обработку через inbox, повторную доставку и некорректные HTTP-запросы.
Задача mise собирает Linux-бинарник и Docker-образ. Testcontainers запускает
сервис и consumer из этого образа вместе с MySQL и RabbitMQ в отдельной сети.
Тестовый MySQL-пароль содержит специальные символы для проверки DSN.
