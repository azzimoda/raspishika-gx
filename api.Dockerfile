FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download
RUN go install github.com/swaggo/swag/cmd/swag@v1.16.6

COPY . .
RUN --mount=type=cache,target=/root/.cache/go-build go generate ./...
RUN --mount=type=cache,target=/root/.cache/go-build CGO_ENABLED=0 go build -v -o api ./cmd/api
RUN --mount=type=cache,target=/root/.cache/go-build CGO_ENABLED=0 go build -v -o fakeapi ./cmd/fakeapi


FROM golang:1.26-alpine

WORKDIR /app

RUN apk add --no-cache ca-certificates

COPY --from=builder /app/api .
COPY --from=builder /app/fakeapi .

EXPOSE 8080

CMD ["./api"]
