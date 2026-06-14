# Tech Stack — «Гляделки» (канонический источник правды)

> Дистиллированный, конкретный тех-стек проекта. Основан на продуктовой спеке (`10-tech-stack.md`) и
> **совпадает с каноном репозитория** (`CLAUDE.md` + скилл `go-backend-conventions`). Спека и репо НЕ
> конфликтуют: репо уже использует PostgreSQL + Redis из спеки. Этот файл фиксирует и те детали, что в
> прозе спеки опущены (HTTP-фреймворк — `Echo`; логгер — zap), чтобы не было «близких коллов».
>
> Источник истины для бэкенда — скилл `go-backend-conventions`. При любом расхождении правит он.

## Бэкенд (Go)
- **Язык:** Go 1.26. Один модуль на всё репо: `github.com/pizdagladki/full`.
- **HTTP:** `github.com/labstack/echo/v4` (Echo) — роутер, middleware, биндинг и валидация запросов.
  Хендлеры `func(c echo.Context) error`; роуты `e.POST("/v1/...", h)`; `validator/v10` подключается как
  `e.Validitor`. Других веб-фреймворков нет.
- **Realtime / signaling:** `github.com/coder/websocket` — обмен SDP/ICE, матчмейкинг, серверный
  арбитраж времени. Контекст-нативный API (`Accept`/`Read`/`Write` берут `context.Context`); конкурентные
  записи безопасны.
- **Основная БД:** PostgreSQL через `github.com/jackc/pgx/v5` + `pgxpool`. SQL пишется руками в слое
  repository, строки маппятся в доменные модели. Денежные/многошаговые операции — в явной транзакции.
  JSONB для гибких полей (напр. мета отвлекалок).
- **Миграции:** `golang-migrate` — парные `migrations/NNNN_<name>.up.sql` / `.down.sql` в сервисе, цель
  `make migrate`.
- **Кэш / координация:** Redis через `github.com/redis/go-redis/v9` — очередь матчмейкинга, кэш горячих
  данных (рейтинги), кулдауны, сессии.
- **Объектное хранилище:** `github.com/minio/minio-go/v7` (MinIO сейчас; тот же API → S3 позже).
- **Авторизация:** Google OAuth через `golang.org/x/oauth2` (+ `.../oauth2/google`); сессия в Redis;
  `auth_middleware.RequireAuth` валидирует сессию; `auth_repository` хранит юзера в Postgres.
- **Платежи:** Stripe через `github.com/stripe/stripe-go` за интерфейсом `PaymentProvider` в слое service
  (альтернативный РФ-провайдер подключается без правки логики покупок).
- **Медиа:** конвертация WebM→MP4 — вызов `ffmpeg` через `os/exec` (не чистый Go).
- **Логирование:** `go.uber.org/zap`. **Валидация:** `github.com/go-playground/validator/v10`
  (конфиг + DTO).
- **Общий код `internal/platform/`:** `logger` (zap), `postgres` (pgxpool), `redis` (go-redis),
  `storage` (minio-go). Сервисы импортируют это и НЕ дублируют.
- **Архитектура сервиса:** слоёная `delivery → service → repository → domain` + `app` (сборка) +
  `config`; зависимости строго внутрь. Раскладка `services/<name>/` с приватным `internal/`. Детали —
  скиллы `go-backend-conventions`, `new-service`, `new-resource`.

## Фронтенд (React)
- **Фреймворк:** React (своя экосистема в `frontend/`, вне Go-модуля).
- **CV-детекция:** MediaPipe Face Mesh / FaceLandmarker, blink по EAR (локально, покадрово).
- **Запись эдита:** canvas + MediaRecorder (WebM), `captureStream` — на лету.
- **Эдиты:** HTML/Canvas/WebGL шаблоны (производятся в Claude Design — зона `[OWNER-DESIGN]`).
- Canvas / CV / WebRTC — в изолированных компонентах за ref'ами.

## Сеть / видео
- **WebRTC P2P** между игроками. **STUN** сразу (публичные серверы). **TURN** (coturn/облачный) —
  бэклог (вторая итерация).

## Инфраструктура
- **DigitalOcean + Docker** (docker-compose: Go-сервисы + Postgres + Redis + MinIO; coturn позже).
  Фронт — статика. **Kubernetes — пост-MVP.**
- Образы сервисов: distroless-static по умолчанию; debian-slim с `ffmpeg` для медиа-сервиса.

## Внешние сервисы
- **AdSense** (баннеры, все экраны кроме батла), **AdMob / Unity Ads** (rewarded video),
  **Telegram-бот** (приём багрепортов), **Stripe** (+ РФ-провайдер позже), **Google OAuth**.

## Сводка одним взглядом

| Слой | Технология |
|---|---|
| Язык бэка | Go 1.26 (`github.com/pizdagladki/full`) |
| HTTP | `Echo` (`labstack/echo/v4`) |
| Signaling / realtime | `coder/websocket` |
| Основная БД | PostgreSQL — `pgx v5` + `pgxpool` |
| Миграции | `golang-migrate` |
| Кэш / очередь / кулдауны / сессии | Redis — `go-redis v9` |
| Объектное хранилище | MinIO — `minio-go v7` → S3 |
| Авторизация | Google OAuth (`x/oauth2`), сессия в Redis |
| Платежи | Stripe (`stripe-go`) за `PaymentProvider` |
| Медиа | WebM→MP4 через `ffmpeg` (`os/exec`) |
| Логи / валидация | `zap` / `validator/v10` |
| Фронтенд | React + MediaPipe (EAR) + canvas/MediaRecorder |
| Видео-связь | WebRTC P2P + STUN (TURN — бэклог) |
| Инфра | DigitalOcean + Docker (K8s — позже) |

---

_Совпадает с `full/CLAUDE.md` и скиллом `go-backend-conventions`. Если копируешь спеку в репо — клади
этот файл в `docs/specs/tech-stack.md`._
