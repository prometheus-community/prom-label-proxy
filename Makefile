PHONY: all
all: check-license build generate test

GITHUB_URL=github.com/openshift/prom-label-proxy
GOOS?=$(shell uname -s | tr A-Z a-z)
GOARCH?=$(subst x86_64,amd64,$(patsubst i%86,386,$(shell uname -m)))
OUT_DIR=_output
BIN?=prom-label-proxy
VERSION?=$(shell cat VERSION)
PKGS=$(shell go list ./... | grep -v /vendor/)
DOCKER_REPO?=quay.io/coreos/prom-label-proxy

.PHONY: check-license
check-license:
	@echo ">> checking license headers"
	@./scripts/check_license.sh

.PHONY: crossbuild
crossbuild:
	@GOOS=darwin ARCH=amd64 $(MAKE) -s build
	@GOOS=linux ARCH=amd64 $(MAKE) -s build
	@GOOS=windows ARCH=amd64 $(MAKE) -s build

.PHONY: build
build:
	@$(eval OUTPUT=$(OUT_DIR)/$(GOOS)/$(GOARCH)/$(BIN))
	@echo ">> building for $(GOOS)/$(GOARCH) to $(OUTPUT)"
	@mkdir -p $(OUT_DIR)/$(GOOS)/$(GOARCH)
	@CGO_ENABLED=0 go build --installsuffix cgo -o $(OUTPUT) $(GITHUB_URL)

.PHONY: container
container:
	docker build -t $(DOCKER_REPO):$(VERSION) .

.PHONY: run-curl-container
run-curl-container:
	@echo 'Example: curl -v -s -k -H "Authorization: Bearer `cat /var/run/secrets/kubernetes.io/serviceaccount/token`" https://kube-rbac-proxy.default.svc:8443/api/v1/query?query=up\&namespace=default'
	kubectl run -i -t krp-curl --image=quay.io/brancz/krp-curl:v0.0.1 --restart=Never --command -- /bin/sh

.PHONY: test
test:
	@echo ">> running all tests"
	@go test $(PKGS)

.PHONY: vendor
vendor:
	go mod tidy
	go mod vendor
	go mod verify

.PHONY: generate
generate: embedmd
	@echo ">> generating examples"
	@./scripts/generate-examples.sh
	@echo ">> generating docs"
	@./scripts/generate-help-txt.sh
	@$(GOPATH)/bin/embedmd -w `find ./ -path ./vendor -prune -o -name "*.md" -print`

.PHONY: embedmd
embedmd:
	@go get github.com/campoy/embedmd
