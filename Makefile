GO ?= go
BIN := trailmark

.PHONY: all build test vet check run clean

all: check build

build:
	$(GO) build -o $(BIN) ./cmd/trailmark

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

check: vet test

run:
	$(GO) run ./cmd/trailmark $(ARGS)

clean:
	rm -f $(BIN)
