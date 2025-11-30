FROM golang:1.25.4-alpine AS builder

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
      -o /app/server \
      ./main.go

FROM alpine:latest

WORKDIR /bin

COPY --from=builder /app/server /bin/server

EXPOSE 8080
EXPOSE 9090

ENTRYPOINT ["/bin/server"]