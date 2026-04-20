# syntax=docker/dockerfile:1.7

FROM golang:1.26-alpine AS builder
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /out/mesh ./cmd/mesh

FROM gcr.io/distroless/static-debian12:nonroot

USER 1337:1337
COPY --from=builder /out/mesh /mesh

ENTRYPOINT ["/mesh"]
