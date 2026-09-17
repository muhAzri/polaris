.PHONY: setup dev up down logs api web mobile build fmt

setup:
	cp -n .env.example .env || true
	cd apps/web && npm install
	cd apps/mobile && flutter pub get

# Full stack in Docker — closest to production.
up:
	docker compose up -d --build

down:
	docker compose down

logs:
	docker compose logs -f

# Local dev with hot reload: infra in Docker, apps run natively.
dev:
	docker compose up -d postgres minio
	@echo "Postgres + MinIO are up. In separate terminals, run:"
	@echo "  make api"
	@echo "  make web"

api:
	cd apps/api && set -a && . ../../.env && set +a && go run ./cmd/server

web:
	cd apps/web && npm run dev

mobile:
	cd apps/mobile && flutter run

build:
	cd apps/api && go build -o bin/server ./cmd/server
	cd apps/web && npm run build

fmt:
	cd apps/api && gofmt -w .
	cd apps/web && npx eslint --fix .
	cd apps/mobile && dart format .
