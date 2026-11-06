.PHONY: build test install uninstall

build:
	go build -o bin/redus ./cmd/redus

test:
	go vet ./... && go test ./...

install:
	./scripts/install.sh

uninstall:
	./scripts/uninstall.sh
