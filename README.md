# Polaris

An open-source LMS inspired by Moodle's feature scope — courses, enrollment,
assignments, grading — rebuilt from scratch on a modern, resource-efficient
stack: clean UX, low infrastructure cost, and a developer experience that
starts with one command.

## Stack

| Layer    | Choice                          |
| -------- | -------------------------------- |
| Web      | Next.js + TypeScript + Tailwind |
| Mobile   | Flutter (Android + iOS)         |
| Backend  | Go (modular monolith)           |
| Database | PostgreSQL                      |
| Files    | S3-compatible object storage (MinIO locally) |
| Deploy   | Docker Compose                  |

```text
Next.js  ──▶  Go API  ──▶  PostgreSQL
                 │
                 └────────▶  S3 / MinIO
```

## Quickstart

```bash
git clone <this-repo>
cd polaris

make setup        # copies .env.example -> .env, installs web deps
make up            # builds and starts postgres, minio, api, web
```

Then open:

- Web: http://localhost:3000
- API health check: http://localhost:8080/api/v1/health
- MinIO console: http://localhost:9001 (user/pass from `.env`)

The API applies its own database migrations on startup — there's no
separate migrate step to run.

## Local development (hot reload)

Running everything in Docker is convenient but slow to iterate against.
For active development, run just the infra in Docker and the apps natively:

```bash
make dev     # starts postgres + minio only
make api     # in one terminal: go run ./cmd/server with live env
make web     # in another terminal: next dev
```

## Project structure

```text
polaris/
├── apps/
│   ├── web/      # Next.js frontend
│   ├── api/      # Go backend
│   └── mobile/   # Flutter mobile app (Android + iOS)
│
├── infra/
│   └── data/    # docker compose volumes (gitignored)
│
├── docker-compose.yml
├── Makefile
└── .env.example
```

There's no `packages/` directory yet on purpose. It gets added the moment
there's real shared code between apps (a generated API client from the
OpenAPI spec, shared UI components, shared types) — not before.

### apps/api

```text
apps/api/
├── cmd/server/          # entrypoint: config, DB connect, migrate, serve
└── internal/
    ├── config/          # env-based configuration
    ├── database/        # pgx pool + embedded SQL migrations
    ├── auth/             # register/login/me, JWT issuing + middleware
    ├── course/           # courses + enrollment
    ├── storage/           # Storage interface + S3/MinIO implementation
    └── httpserver/       # route wiring
```

Each domain lives in its own `internal/` package so a feature can grow
without becoming a tangle, and can be extracted into its own service later
if it ever genuinely needs to scale independently — nothing here assumes
that day will come.

### apps/mobile

```text
apps/mobile/
├── android/   # native Android project
├── ios/       # native iOS project
├── lib/       # Dart app code (entrypoint: lib/main.dart)
└── test/
```

A standard `flutter create` scaffold, package `com.zrifapps.polaris_mobile`,
targeting Android and iOS only. Points at the same Go API as the web app.

```bash
cd apps/mobile
flutter pub get
flutter run
```

## API reference (v0.1)

| Method | Path                      | Auth | Description          |
| ------ | ------------------------- | ---- | --------------------- |
| GET    | `/api/v1/health`          | –    | Liveness check        |
| POST   | `/api/v1/auth/register`   | –    | Create an account     |
| POST   | `/api/v1/auth/login`      | –    | Get a JWT             |
| GET    | `/api/v1/auth/me`         | ✅   | Current user          |
| GET    | `/api/v1/courses`         | –    | List courses          |
| POST   | `/api/v1/courses`         | ✅   | Create a course       |
| POST   | `/api/v1/courses/{id}/enroll` | ✅ | Enroll in a course |

Authenticated requests send `Authorization: Bearer <token>`.

## Roadmap

- [x] v0.1 — Auth, courses, enrollment
- [ ] v0.2 — Assignments, submissions
- [ ] v0.3 — Quizzes, grading
- [ ] v0.4 — File uploads (assignments, course materials) via presigned URLs
- [ ] v0.5 — Notifications
- [ ] v0.6 — Flutter mobile app (offline-first course viewing, scaffolded)
- [ ] v1.0 — Plugin system, self-hosting docs, first tagged release

## License

MIT — see [LICENSE](LICENSE).
