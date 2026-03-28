# ABOUTME: Multi-stage Dockerfile for the keytun relay server.
# ABOUTME: Builds the Go binary and runs it in a minimal distroless container.

FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o keytun .

FROM --platform=linux/amd64 gcr.io/distroless/static-debian12

COPY --from=builder /app/keytun /keytun

ENTRYPOINT ["/keytun", "relay", "--port", "8080"]
