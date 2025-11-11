FROM golang:1.22-alpine AS builder

WORKDIR /src

# Копируем go.mod/go.sum и локальную зависимость, чтобы эффективнее кэшировать загрузку модулей.
COPY go.mod go.sum ./
COPY third_party/max-bot-api-client-go ./third_party/max-bot-api-client-go
RUN go mod download

# Копируем остальной исходный код и собираем бинарник.
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /bin/max-bot ./cmd/bot

FROM alpine:3.20

RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /bin/max-bot ./max-bot

EXPOSE 8080
ENTRYPOINT ["./max-bot"]
