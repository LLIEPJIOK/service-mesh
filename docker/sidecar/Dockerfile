FROM golang:1.24.3-alpine AS builder

WORKDIR /app

COPY go.mod ./go.mod

RUN GOOS=linux \
    GOARCH=amd64 \
    go mod download

COPY . .

RUN GOOS=linux \
      GOARCH=amd64 \
      CGO_ENABLED=0 \
      go build \
      -o /app/sidecar \
      ./cmd/sidecar/main.go

FROM alpine:latest

WORKDIR /bin

COPY --from=builder /app/sidecar /bin/sidecar

EXPOSE 8080
ENTRYPOINT ["/bin/sidecar"]