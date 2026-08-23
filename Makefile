.PHONY: build test vet e2e fmt tidy clean

VERSION ?= $(shell (git describe --tags --always --dirty 2>/dev/null || echo dev) | sed 's/^v//')
LDFLAGS := -X github.com/vshuraeff/pushover/internal/cli.Version=$(VERSION)

build:
	go build -ldflags "$(LDFLAGS)" ./cmd/povr

test:
	go test ./...

vet:
	go vet ./...

e2e:
	go test -tags e2e ./...

fmt:
	gofmt -w cmd internal

tidy:
	go mod tidy

clean:
	rm -f povr
