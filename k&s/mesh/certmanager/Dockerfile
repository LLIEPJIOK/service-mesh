# syntax=docker/dockerfile:1.7

ARG GOARCH=amd64
FROM --platform=linux/${GOARCH} golang:1.26-alpine AS builder
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal

RUN CGO_ENABLED=0 GOOS=linux GOARCH=${GOARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/certmanager ./cmd/certmanager

FROM --platform=linux/${GOARCH} gcr.io/distroless/static-debian12:nonroot

USER 1337:1337
COPY --from=builder /out/certmanager /certmanager

EXPOSE 8080

ENTRYPOINT ["/certmanager"]