.PHONY: up down migrate-up migrate-down api consumer relay grpc test tidy gen install-tools test-e2e ui-dev ui-build

up:
	docker compose -f ../dev.compose.yml up -d

down:
	docker compose -f ../dev.compose.yml down

migrate-up:
	go run ./cmd/migrate -cmd up

migrate-down:
	go run ./cmd/migrate -cmd down

api:
	go run ./cmd/api

consumer:
	go run ./cmd/consumer

relay:
	go run ./cmd/outbox-relay

grpc:
	go run ./cmd/grpc

test:
	go test ./...

tidy:
	go mod tidy

install-tools:
	go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.4.1

gen:
	go generate ./...

gen-ui:
	cd ../web-ui && npm install && npm run gen

ui-dev:
	cd ../web-ui && npm install && npm run dev

ui-build:
	cd ../web-ui && npm install && npm run build

refresh-music:
	@MINIO_ENDPOINT=$${MINIO_ENDPOINT:-localhost:9000} \
		MINIO_ACCESS_KEY=$${MINIO_ACCESS_KEY:-minioadmin} \
		MINIO_SECRET_KEY=$${MINIO_SECRET_KEY:-minioadmin} \
		MINIO_BUCKET=$${MINIO_BUCKET:-webcomics} \
		python3 ../scripts/refresh_music_library.py

test-e2e:
	./scripts/test-e2e.sh

test-integration:
	PG_TEST_DSN=$${PG_TEST_DSN:-postgres://app:app@localhost:5433/app?sslmode=disable} \
	  go test -tags=integration ./internal/app/command/pipeline/...
