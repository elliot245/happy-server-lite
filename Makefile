.PHONY: build run test race tidy docker-build

BINARY_NAME=happy-server-lite
MAIN_PATH=./cmd/server

build:
	CGO_ENABLED=0 go build -ldflags="-w -s" -o $(BINARY_NAME) $(MAIN_PATH)

run:
	go run $(MAIN_PATH)

test:
	go test ./...

race:
	go test -race ./...

tidy:
	go mod tidy

docker-build:
	docker build -t happy-server-lite .
