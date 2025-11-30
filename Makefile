.PHONY: run
run: 
	meshctl init
	sleep 40
	meshctl apply -f ./example/manifests/counter.yaml
	meshctl apply -f ./example/manifests/test.yaml

.PHONY: prepare
prepare: push install

.PHONY: push
push: 
	cd ./cdocker && make push
	cd ./control-plane && make push
	cd ./example/apps/test && make push
	cd ./example/apps/counter && make push
	cd ./sidecar && make push

.PHONY: install
install: 
	cd ./meshctl && go install .

.PNONY: clean
clean: 
	meshctl destroy

.PNONY: hard-clean
hard-clean:
	docker ps -a -q --filter "label=com.docker.compose.project=control-plane" | xargs --no-run-if-empty docker rm -f