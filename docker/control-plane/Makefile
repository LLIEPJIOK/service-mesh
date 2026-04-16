ifneq (,$(wildcard .env))
	include .env
	export $(shell sed 's/=.*//' .env)
endif

.PHONY: run
run:
	go run cmd/mesh/main.go

.PHONY: push
push:
	docker build . -t lliepjiok/control-plane:latest
	docker push lliepjiok/control-plane:latest
