# Polaris

An open-source LMS modelled closely on Moodle's feature scope — courses, roles
and permissions, enrolment, gradebook, groups, calendar, and soon assignments and
quizzes — rebuilt from scratch on a modern, resource-efficient
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
├── e2e/                 # end-to-end parity tests (need a throwaway database)
└── internal/
    ├── app/             # wires services and handlers into one router
    ├── config/          # env-based configuration
    ├── database/        # pgx pool + embedded SQL migrations
    ├── auth/            # register/login/me, JWT issuing + middleware
    ├── rbac/            # roles, capabilities, per-context assignments, overrides
    ├── course/          # courses, categories, sections, enrolment
    ├── coursemodule/    # generic activity/resource attachment + edit/move/delete
    ├── content/         # label, page, url, resource, folder, book
    ├── gradebook/       # category tree, items, grades, aggregation, reports
    ├── groups/          # groups + groupings
    ├── calendar/        # course/user/site events, repeats, iCal export
    ├── eventbus/        # in-process pub/sub + event_log
    ├── storage/         # Storage interface + S3/MinIO implementation
    └── httpserver/      # route wiring
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

## API reference

Every route is registered in `apps/api/internal/httpserver/router.go`, next to
the capability it requires. Authenticated requests send
`Authorization: Bearer <token>`. Permissions are checked with Moodle-style
roles: a role is assigned to a user at a *context* (site → category → course
→ module) and permissions are resolved down that path, with per-context
overrides. A user's role inside a course comes from their enrolment, not from
`users.role`.

| Area | Routes (all under `/api/v1`) |
| ---- | ---------------------------- |
| Auth | `POST /auth/register`, `POST /auth/login`, `GET /auth/me` |
| Courses | `GET/POST /courses`, `GET /courses/available`, `GET/PUT/DELETE /courses/{id}`, `GET /courses/{id}/my-capabilities` |
| Categories | `GET/POST /categories`, `PUT/DELETE /categories/{id}` (`?move_to=` relocates contents) |
| Sections | `GET/POST /courses/{id}/sections`, `PATCH/DELETE /sections/{id}` |
| Enrolment | `GET /courses/{id}/participants`, `POST /courses/{id}/enrolments`, `PATCH/DELETE /courses/{id}/enrolments/{userId}`, `GET/PUT /courses/{id}/self-enrolment`, `POST /courses/{id}/enroll` |
| Roles | `GET /roles`, `/admin/roles`, `/admin/capabilities`, `/admin/role-assignments`; per context: `/courses/{id}/role-assignments`, `/courses/{id}/role-overrides` (same under `/categories/{id}`) |
| Content | `GET /courses/{id}/content`, `POST /sections/{id}/modules/{label\|page\|url\|resource\|folder\|book}`, `PATCH/DELETE /modules/{id}`, `PUT /modules/{id}/content`, folder files and book chapters under `/modules/{id}/…` |
| Gradebook | `/courses/{id}/gradebook/{categories,items,mine,report,users/{userId}}`, `PATCH/DELETE /gradebook/{categories,items}/{id}`, `POST /gradebook/items/{id}/grades/{userId}`, grade history |
| Groups | `/courses/{id}/groups` (+ `/auto`), `/courses/{id}/groupings`, `PATCH/DELETE /groups/{id}`, group members, grouping contents |
| Calendar | `/courses/{id}/events`, `/users/me/events`, `GET /users/me/calendar.ics`, `PATCH/DELETE /events/{id}`, `POST /admin/events` |

### Tests

```bash
cd apps/api && go test ./...        # aggregation and calendar unit tests
```

`apps/api/e2e` builds the whole API in-process and drives it over HTTP
through the behaviours that should match Moodle (per-course roles,
inheritance, overrides, hidden content, gradebook totals, groups, calendar).
It is skipped unless `E2E_DATABASE_URL` is set, and it **drops the whole
schema** of that database, so it refuses any database whose name does not
contain `e2e` or `test`:

```bash
docker compose exec -T postgres psql -U polaris postgres -c "create database polaris_e2e"
cd apps/api
E2E_DATABASE_URL='postgres://polaris:polaris@localhost:5433/polaris_e2e?sslmode=disable' go test ./e2e/
```

## Roadmap

The plan for reaching Moodle core parity, phase by phase with dependencies
and exit criteria, lives in `ROADMAP.md` one directory up from this folder.
Foundation, static content, gradebook, groups and calendar are done and audited;
assignments are next.

## License

MIT — see [LICENSE](LICENSE).
