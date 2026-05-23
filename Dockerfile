FROM golang:1.26-bookworm

RUN apt-get update && apt-get install -y nodejs chromium

WORKDIR /app

RUN go run github.com/playwright-community/playwright-go/cmd/playwright@v0.5200.0 install chromium chromium-headless-shell --with-deps
RUN rm -rf /var/lib/apt/lists/*

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -v -o raspishika ./cmd/bot

ENTRYPOINT ["/app/raspishika"]
