PWD := $(shell pwd)
GOPATH := $(shell go env GOPATH)
LDFLAGS := $(shell go run buildscripts/gen-ldflags.go)

GOOS := $(shell go env GOOS)
GOOSALT ?= 'linux'
ifeq ($(GOOS),'darwin')
  GOOSALT = 'mac'
endif

BUILD_LDFLAGS := '$(LDFLAGS)'

all: build

checks:
	@echo "Checking dependencies"
	@(env bash $(PWD)/buildscripts/checkdeps.sh)

getdeps:
	@mkdir -p ${GOPATH}/bin
	@which golangci-lint 1>/dev/null || (echo "Installing golangci-lint" && curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(GOPATH)/bin v1.27.0)

crosscompile:
	@(env bash $(PWD)/buildscripts/cross-compile.sh)

verifiers: getdeps lint

lint:
	@echo "Running $@ check"
	@GO111MODULE=on ${GOPATH}/bin/golangci-lint cache clean
	@GO111MODULE=on ${GOPATH}/bin/golangci-lint run --build-tags kqueue --timeout=10m --config ./.golangci.yml

test: verifiers build
	@echo "Running unit tests"
	@GO111MODULE=on CGO_ENABLED=0 go test -tags kqueue ./... 1>/dev/null

coverage: build
	@echo "Running all coverage for MinIO"
	@(env bash $(PWD)/buildscripts/go-coverage.sh)

# Builds mc locally.
build: checks
	@echo "Building minfs binary to './minfs'"
	@GO111MODULE=on CGO_ENABLED=0 go build -tags kqueue --ldflags $(BUILD_LDFLAGS) -o $(PWD)/minfs

# Builds MinFS and installs it to $GOPATH/bin.
install: build
	@sudo /usr/bin/install -m 755 minfs /sbin/minfs && echo "Installing minfs binary to '/sbin/minfs'"
	@sudo /usr/bin/install -m 755 mount.minfs /sbin/mount.minfs && echo "Installing '/sbin/mount.minfs'"
	@echo "Installing man pages"
	@sudo /usr/bin/install -m 644 docs/minfs.8 /usr/share/man/man8/minfs.8
	@sudo /usr/bin/install -m 644 docs/mount.minfs.8 /usr/share/man/man8/mount.minfs.8
	@echo "Installation successful. To learn more, try \"minfs --help\"."

clean:
	@echo "Cleaning up all the generated files"
	@find . -name '*.test' | xargs rm -fv
	@find . -name '*~' | xargs rm -fv
	@rm -rvf minfs
	@rm -rvf build
	@rm -rvf release

