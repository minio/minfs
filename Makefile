PWD := $(shell pwd)
GOPATH := $(shell go env GOPATH)

GOOS := $(shell go env GOOS)
BUILD_LDFLAGS := '-s -w'

all: build

checks: ## check dependencies
	@echo "Checking dependencies"
	@(env bash $(PWD)/buildscripts/checkdeps.sh)

getdeps: ## get necessary dependencies
	@mkdir -p ${GOPATH}/bin
	@which golangci-lint 1>/dev/null || (echo "Installing golangci-lint" && curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(GOPATH)/bin v1.27.0)

crosscompile: ## check cross-compilation works
	@(env bash $(PWD)/buildscripts/cross-compile.sh)

help: ## print this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' Makefile | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

verifiers: getdeps lint

lint: ## run linters
	@echo "Running $@ check"
	@GO111MODULE=on ${GOPATH}/bin/golangci-lint cache clean
	@GO111MODULE=on ${GOPATH}/bin/golangci-lint run --build-tags kqueue --timeout=10m --config ./.golangci.yml

test: verifiers build
	@echo "Running unit tests"
	@GO111MODULE=on CGO_ENABLED=0 go test -tags kqueue ./... 1>/dev/null

coverage: build
	@echo "Running all coverage for MinIO"
	@(env bash $(PWD)/buildscripts/go-coverage.sh)

build: checks ## builds MinFS locally
	@echo "Building minfs binary to './minfs'"
	@GO111MODULE=on CGO_ENABLED=0 go build -tags kqueue --ldflags $(BUILD_LDFLAGS) -o $(PWD)/minfs

install: build ## builds MinFS and installs it to $GOPATH/bin
	@sudo /usr/bin/install -m 755 minfs /sbin/minfs && echo "Installing minfs binary to '/sbin/minfs'"
	@sudo /usr/bin/install -m 755 mount.minfs /sbin/mount.minfs && echo "Installing '/sbin/mount.minfs'"
	@echo "Installing man pages"
	@sudo /usr/bin/install -m 644 docs/minfs.8 /usr/share/man/man8/minfs.8
	@sudo /usr/bin/install -m 644 docs/mount.minfs.8 /usr/share/man/man8/mount.minfs.8
	@echo "Installation successful. To learn more, try \"minfs --help\"."

clean: ## clean all temporary files
	@echo "Cleaning up all the generated files"
	@find . -name '*.test' | xargs rm -fv
	@find . -name '*~' | xargs rm -fv
	@rm -rvf minfs
	@rm -rvf build
	@rm -rvf release

