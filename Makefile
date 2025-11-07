GO ?= go
OUTPUT ?= bot

.PHONY: run build

run:
	$(GO) run ./cmd/bot

build:
	$(GO) build -o $(OUTPUT) ./cmd/bot
