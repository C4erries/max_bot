FROM golang:1.25-alpine AS builder

WORKDIR /src

# Copy module files first to leverage layer caching.
COPY go.mod go.sum ./
RUN go mod download

# Copy the project sources and build the binary.
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /bin/max-bot ./cmd/bot

FROM gcr.io/distroless/static-debian12

WORKDIR /app
COPY --from=builder /bin/max-bot ./max-bot

EXPOSE 8080
ENTRYPOINT ["./max-bot"]

