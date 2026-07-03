.PHONY: run migrate-up migrate-down migrate-create test test-integration lint compose-up compose-down tidy

run:
	go run ./cmd/skillhub

migrate-up:
	go run ./cmd/migrate up

migrate-down:
	go run ./cmd/migrate down

migrate-create:
	@test -n "$(NAME)" || (echo "Usage: make migrate-create NAME=foo" && exit 1)
	go run ./cmd/migrate create $(NAME)

test:
	go test ./...

test-integration:
	go test -tags integration ./...

lint:
	golangci-lint run ./...

compose-up:
	docker compose -f deployments/docker-compose.yml up -d

compose-down:
	docker compose -f deployments/docker-compose.yml down

tidy:
	go mod tidy
