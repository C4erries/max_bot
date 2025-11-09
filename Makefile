GO ?= go
OUTPUT ?= bot
TOKEN ?= f9LHodD0cOJTo2jim5VSB3XegdV5eYOa0orYqub31xNn68MMqVnwDM6fLsTuQXINU4uCaQsF4nh6883oZEB9
LOG_LEVEL ?= debug
.PHONY: run build

run:
	LOG_LEVEL=$(LOG_LEVEL) BOT_TOKEN=$(TOKEN) $(GO) run ./cmd/bot

build:
	$(GO) build -o $(OUTPUT) ./cmd/bot
