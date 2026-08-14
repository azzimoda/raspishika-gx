FROM golang:1.26-alpine

RUN apk add --no-cache chromium gcc musl-dev

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN --mount=type=cache,target=/root/.cache/go-build CGO_ENABLED=1 go build -v -o bot ./cmd/bot

ENTRYPOINT ["/app/bot"]
