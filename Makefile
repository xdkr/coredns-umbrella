COREDNS_VERSION ?= v1.14.7
COREDNS_COMMIT ?= 427fc80ed9ca47f354585eb30a3f1332950856c4
COREDNS_REPOSITORY := https://github.com/coredns/coredns.git
COREDNS_DIRECTORY := .coredns-build
BINARY := coredns
PLUGIN := umbrella:github.com/xdkr/coredns-umbrella
E2E_COMPOSE := docker compose -f e2e/compose.yaml

.PHONY: build clean test test-e2e test-e2e-ipv4 test-e2e-ipv6

build:
	rm -rf -- "$(COREDNS_DIRECTORY)"
	git -c advice.detachedHead=false clone --branch "$(COREDNS_VERSION)" --depth 1 "$(COREDNS_REPOSITORY)" "$(COREDNS_DIRECTORY)"
	test "$$(git -C "$(COREDNS_DIRECTORY)" rev-parse HEAD)" = "$(COREDNS_COMMIT)"
	grep -qx 'forward:forward' "$(COREDNS_DIRECTORY)/plugin.cfg"
	awk '/^forward:forward$$/ { print "$(PLUGIN)" } { print }' "$(COREDNS_DIRECTORY)/plugin.cfg" > "$(COREDNS_DIRECTORY)/plugin.cfg.tmp"
	mv "$(COREDNS_DIRECTORY)/plugin.cfg.tmp" "$(COREDNS_DIRECTORY)/plugin.cfg"
	awk 'previous == "$(PLUGIN)" && $$0 == "forward:forward" { found = 1 } { previous = $$0 } END { exit !found }' "$(COREDNS_DIRECTORY)/plugin.cfg"
	cd "$(COREDNS_DIRECTORY)" && go mod edit -require=github.com/xdkr/coredns-umbrella@v0.0.0 -replace=github.com/xdkr/coredns-umbrella=..
	$(MAKE) -C "$(COREDNS_DIRECTORY)" GOFLAGS="-buildvcs=false" gen
	$(MAKE) -C "$(COREDNS_DIRECTORY)" GOFLAGS="-buildvcs=false" GITCOMMIT="$(COREDNS_VERSION)-umbrella" BUILDOPTS="-trimpath" coredns
	cp "$(COREDNS_DIRECTORY)/coredns" "$(BINARY)"
	./"$(BINARY)" -plugins | grep -qx umbrella

test:
	go test ./...

test-e2e:
	$(MAKE) test-e2e-ipv4
	$(MAKE) test-e2e-ipv6

test-e2e-ipv4:
	@status=0; \
	$(E2E_COMPOSE) --profile ipv4 up --build --abort-on-container-exit --exit-code-from client-ipv4 || status=$$?; \
	$(E2E_COMPOSE) --profile ipv4 down --volumes --remove-orphans; \
	exit $$status

test-e2e-ipv6:
	@status=0; \
	$(E2E_COMPOSE) --profile ipv6 up --build --abort-on-container-exit --exit-code-from client-ipv6 || status=$$?; \
	$(E2E_COMPOSE) --profile ipv6 down --volumes --remove-orphans; \
	exit $$status

clean:
	rm -rf -- "$(COREDNS_DIRECTORY)" "$(BINARY)"
