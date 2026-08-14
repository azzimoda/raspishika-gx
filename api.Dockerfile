FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download
RUN go install github.com/swaggo/swag/cmd/swag@v1.16.6

COPY . .
RUN --mount=type=cache,target=/root/.cache/go-build go generate ./...
RUN --mount=type=cache,target=/root/.cache/go-build CGO_ENABLED=0 go build -v -o api ./cmd/api
RUN --mount=type=cache,target=/root/.cache/go-build CGO_ENABLED=0 go build -v -o fakeapi ./cmd/fakeapi


FROM golang:1.26-bookworm

WORKDIR /app

RUN apt-get update && apt-get install -y nodejs chromium curl

RUN go run github.com/mxschmitt/playwright-go/cmd/playwright@v0.6100.0 install chromium chromium-headless-shell --with-deps
RUN rm -rf /var/lib/apt/lists/*

COPY --from=builder /app/api .
COPY --from=builder /app/fakeapi .

EXPOSE 8080

CMD ["./api"]
