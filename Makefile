.PHONY: build test lint docker-build helm-lint all

all: build

## build: compile operator and migrator
build:
	cd operator && go build ./...
	cd migrator && go build ./...

## test: run operator and migrator tests
test:
	cd operator && make test
	cd migrator && go test ./... -v

## lint: lint operator and migrator
lint:
	cd operator && golangci-lint run ./...
	cd migrator && golangci-lint run ./...

## docker-build: build both container images locally
docker-build:
	docker build -t hermes-operator:dev ./operator
	docker build -t hermes-migrator:dev ./migrator

## helm-lint: lint the Helm chart
helm-lint:
	helm lint charts/hermes
