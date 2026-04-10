.PHONY: dev test migrate-up migrate-down build

dev:
	docker compose up -d postgres firecrawl
	cd backend && go run ./cmd/server/

test:
	docker compose up -d postgres-test
	cd backend && TEST_DATABASE_URL="postgres://hmys:hmys_test@localhost:5433/howmuchyousay_test?sslmode=disable" go test ./... -v

migrate-up:
	cd backend && go run -tags migrate ./cmd/server/ -migrate up

migrate-down:
	cd backend && go run -tags migrate ./cmd/server/ -migrate down

build:
	cd backend && go build -o ../bin/server ./cmd/server/
	cd backend && go build -o ../bin/crawler ./cmd/crawler/
