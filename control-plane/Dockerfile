FROM golang:1.24.2-alpine AS builder

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
      -o /app/mesh \
      ./cmd/mesh/main.go

FROM alpine:latest

WORKDIR /bin

COPY --from=builder /app/mesh /bin/mesh

EXPOSE 8080
ENTRYPOINT ["/bin/mesh"]