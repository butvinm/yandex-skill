VERSION := $(shell git describe --tags --always 2>/dev/null || echo dev)
LDFLAGS := -ldflags "-X main.version=$(VERSION)"
PKG     := ./cmd/yandex-cli

.PHONY: build install test vet fmt lint clean

build:
	go build $(LDFLAGS) -o bin/yandex-cli $(PKG)

install:
	go install $(LDFLAGS) $(PKG)

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -s -w .

lint:
	golangci-lint run

clean:
	rm -rf bin/
