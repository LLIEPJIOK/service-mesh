ifneq (,$(wildcard .env))
	include .env
	export $(shell sed 's/=.*//' .env)
endif

.PHONY: run
run:
	go run cmd/sidecar/main.go

.PHONY: push
push:
	docker build . -t lliepjiok/sidecar:latest
	docker push lliepjiok/sidecar:latest