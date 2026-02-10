BINARY    := resemble
MODULE    := github.com/splusq/resemble
VERSION   ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS   := -ldflags="-s -w -X main.version=$(VERSION)"
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64

.PHONY: build test lint vet clean install run smoke dist

build:
	go build $(LDFLAGS) -o $(BINARY) ./cmd/resemble/

install:
	go install $(LDFLAGS) ./cmd/resemble/

test:
	go test ./... -race -count=1

test-v:
	go test ./... -race -count=1 -v

vet:
	go vet ./...

lint: vet
	@which golangci-lint >/dev/null 2>&1 && golangci-lint run || echo "golangci-lint not installed, skipping"

clean:
	rm -f $(BINARY)
	rm -rf dist/

run: build
	./$(BINARY) start $(ARGS)

smoke: build
	bash scripts/smoke.sh

dist:
	@mkdir -p dist
	@for platform in $(PLATFORMS); do \
		os=$${platform%/*}; \
		arch=$${platform#*/}; \
		output=dist/$(BINARY)-$$os-$$arch; \
		[ "$$os" = "windows" ] && output=$$output.exe; \
		echo "Building $$os/$$arch..."; \
		GOOS=$$os GOARCH=$$arch go build $(LDFLAGS) -o $$output ./cmd/resemble/; \
	done

docker:
	docker build -t $(BINARY) .
