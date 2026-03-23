APP_NAME := tecora-api

.PHONY: run test build tidy

run:
	go run ./cmd/api

test:
	go test ./...

build:
	go build -o bin/$(APP_NAME) ./cmd/api

tidy:
	go mod tidy
