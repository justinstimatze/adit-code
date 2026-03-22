FROM golang:1.25-bookworm AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o /adit ./cmd/adit

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates git && rm -rf /var/lib/apt/lists/*
COPY --from=builder /adit /usr/local/bin/adit
ENTRYPOINT ["adit"]
