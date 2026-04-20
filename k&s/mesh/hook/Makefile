GO ?= go
DOCKER ?= docker

DOCKERHUB_NAMESPACE ?= lliepjiok
IMAGE_NAME ?= mesh-hook
VERSION ?= v0.1.0

IMAGE := $(DOCKERHUB_NAMESPACE)/$(IMAGE_NAME)
BINARY := bin/hook
CMD_PACKAGE := ./cmd/hook
GOFILES := $(shell find cmd internal -type f -name '*.go')

.PHONY: help fmt vet test build clean docker-build docker-push-version docker-push-latest docker-push docker-build-push

help:
	@echo "Available targets:"
	@echo "  fmt                - Run gofmt on source files"
	@echo "  vet                - Run go vet"
	@echo "  test               - Run go test"
	@echo "  build              - Build hook binary"
	@echo "  clean              - Remove local build artifacts"
	@echo "  docker-build       - Build hook image (version + latest tags)"
	@echo "  docker-push        - Push both version and latest tags"
	@echo "  docker-build-push  - Build image and push both tags"
	@echo ""
	@echo "Variables:"
	@echo "  DOCKERHUB_NAMESPACE=$(DOCKERHUB_NAMESPACE)"
	@echo "  IMAGE_NAME=$(IMAGE_NAME)"
	@echo "  VERSION=$(VERSION)"

fmt:
	@if [ -n "$(GOFILES)" ]; then gofmt -w $(GOFILES); fi

vet:
	$(GO) vet ./...

test:
	$(GO) test ./...

build:
	@mkdir -p bin
	$(GO) build -o $(BINARY) $(CMD_PACKAGE)

clean:
	rm -rf bin

docker-build:
	$(DOCKER) build -t $(IMAGE):$(VERSION) -t $(IMAGE):latest .

docker-push-version:
	$(DOCKER) push $(IMAGE):$(VERSION)

docker-push-latest:
	$(DOCKER) push $(IMAGE):latest

docker-push: docker-push-version docker-push-latest

docker-build-push: docker-build docker-push
