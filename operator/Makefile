ENVTEST_K8S_VERSION ?= 1.28.0
LOCALBIN ?= $(shell pwd)/.bin

.PHONY: tidy build test lint envtest

tidy:
	go mod tidy

build:
	go build ./...

test: envtest
	KUBEBUILDER_ASSETS="$(shell $(LOCALBIN)/setup-envtest use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" \
	go test ./... -v

lint:
	golangci-lint run ./...

envtest:
	@mkdir -p $(LOCALBIN)
	@test -s $(LOCALBIN)/setup-envtest || GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest
