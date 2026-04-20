DOCKER ?= docker

DOCKERHUB_NAMESPACE ?= lliepjiok
IMAGE_NAME ?= iptables-init
VERSION ?= v0.1.0

IMAGE := $(DOCKERHUB_NAMESPACE)/$(IMAGE_NAME)

.PHONY: help docker-build docker-push-version docker-push-latest docker-push docker-build-push

help:
	@echo "Available targets:"
	@echo "  docker-build       - Build iptables-init image (version + latest tags)"
	@echo "  docker-push        - Push both version and latest tags"
	@echo "  docker-build-push  - Build image and push both tags"
	@echo ""
	@echo "Variables:"
	@echo "  DOCKERHUB_NAMESPACE=$(DOCKERHUB_NAMESPACE)"
	@echo "  IMAGE_NAME=$(IMAGE_NAME)"
	@echo "  VERSION=$(VERSION)"

docker-build:
	$(DOCKER) build -t $(IMAGE):$(VERSION) -t $(IMAGE):latest .

docker-push-version:
	$(DOCKER) push $(IMAGE):$(VERSION)

docker-push-latest:
	$(DOCKER) push $(IMAGE):latest

docker-push: docker-push-version docker-push-latest

docker-build-push: docker-build docker-push