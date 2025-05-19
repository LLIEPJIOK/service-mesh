.PHONY: run
run:
	docker ps -a -q \
  --filter "label=com.docker.compose.project=control-plane" \
	| xargs --no-run-if-empty docker rm -f
	meshctl plane
	meshctl deploy -d test/ -n test -r 3
	meshctl deploy -d counter/ -n counter
	meshctl prom -c ./prometheus.yml
