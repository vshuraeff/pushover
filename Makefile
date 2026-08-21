.PHONY: build test vet e2e fmt tidy clean

build:
	go build ./cmd/povr

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
