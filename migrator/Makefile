.PHONY: tidy build test lint

tidy:
	go mod tidy

build:
	go build ./...

test:
	go test ./... -v

lint:
	golangci-lint run ./...
