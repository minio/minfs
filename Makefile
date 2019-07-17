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
	@which golint 1>/dev/null || (echo "Installing golint" && go get -u golang.org/x/lint/golint)
	@which staticcheck 1>/dev/null || (echo "Installing staticcheck" && wget --quiet -O ${GOPATH}/bin/staticcheck https://github.com/dominikh/go-tools/releases/download/2019.1/staticcheck_linux_amd64 && chmod +x ${GOPATH}/bin/staticcheck)
	@which misspell 1>/dev/null || (echo "Installing misspell" && wget --quiet https://github.com/client9/misspell/releases/download/v0.3.4/misspell_0.3.4_${GOOSALT}_64bit.tar.gz && tar xf misspell_0.3.4_${GOOSALT}_64bit.tar.gz && mv misspell ${GOPATH}/bin/misspell && chmod +x ${GOPATH}/bin/misspell && rm -f misspell_0.3.4_${GOOSALT}_64bit.tar.gz)

crosscompile:
	@(env bash $(PWD)/buildscripts/cross-compile.sh)

verifiers: getdeps vet fmt lint staticcheck spelling

vet:
	@echo "Running $@"
	@GO111MODULE=on go vet github.com/minio/minfs/...

fmt:
	@echo "Running $@"
	@GO111MODULE=on gofmt -d cmd/
	@GO111MODULE=on gofmt -d fs/
	@GO111MODULE=on gofmt -d meta/

lint:
	@echo "Running $@"
	@GO111MODULE=on ${GOPATH}/bin/golint -set_exit_status github.com/minio/minfs/cmd/...
	@GO111MODULE=on ${GOPATH}/bin/golint -set_exit_status github.com/minio/minfs/fs/...
	@GO111MODULE=on ${GOPATH}/bin/golint -set_exit_status github.com/minio/minfs/meta/...

staticcheck:
	@echo "Running $@"
        @GO111MODULE=on ${GOPATH}/bin/staticcheck github.com/minio/minfs/cmd/...
        @GO111MODULE=on ${GOPATH}/bin/staticcheck github.com/minio/minfs/fs/...
        @GO111MODULE=on ${GOPATH}/bin/staticcheck github.com/minio/minfs/meta/...

spelling:
        @GO111MODULE=on ${GOPATH}/bin/misspell -locale US -error `find cmd/`
        @GO111MODULE=on ${GOPATH}/bin/misspell -locale US -error `find fs/`
        @GO111MODULE=on ${GOPATH}/bin/misspell -locale US -error `find meta/`
        @GO111MODULE=on ${GOPATH}/bin/misspell -locale US -error `find docs/`

test: verifiers build
	@echo "Running unit tests"
	@GO111MODULE=on CGO_ENABLED=0 go test -tags kqueue ./... 1>/dev/null

coverage: build
	@echo "Running all coverage for MinIO"
	@(env bash $(PWD)/buildscripts/go-coverage.sh)

# Builds mc locally.
build: checks
	@echo "Building minfs binary to './minfs'"
	@GO111MODULE=on GO_FLAGS="" CGO_ENABLED=0 go build -tags kqueue --ldflags $(BUILD_LDFLAGS) -o $(PWD)/minfs

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

