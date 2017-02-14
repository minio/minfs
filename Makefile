LDFLAGS := $(shell go run buildscripts/gen-ldflags.go)
DIRS := *.go fs/**.go cmd/**.go meta/**.go

all: gomake-all

checks:
	@echo "Checking deps:"
	@(env bash buildscripts/checkdeps.sh)
	@(env bash buildscripts/checkgopath.sh)

getdeps: checks
	@go get github.com/golang/lint/golint && echo "Installed golint:"
	@go get github.com/fzipp/gocyclo && echo "Installed gocyclo:"
	@go get github.com/remyoudompheng/go-misc/deadcode && echo "Installed deadcode:"
	@go get github.com/client9/misspell/cmd/misspell && echo "Installed misspell:"

# verifiers: getdeps vet fmt lint cyclo deadcode
verifiers: vet lint cyclo deadcode spelling # todo

todo:
	@echo "Running $@:"
	@$(foreach DIR, $(DIRS), fgrep -i todo $(DIR)||true;)

vet:
	@echo "Running $@:"
	@$(foreach DIR, $(DIRS), go tool vet -all $(DIR);)
	@$(foreach DIR, $(DIRS), go tool vet -shadow=true $(DIR);)

spelling:
	@$(foreach DIR, $(DIRS), ${GOPATH}/bin/misspell $(DIR);)

lint:
	@echo "Running $@:"
	@$(foreach DIR, $(DIRS), $(GOPATH)/bin/golint $(DIR);)

cyclo:
	@echo "Running $@:"
	@$(foreach DIR, $(DIRS), $(GOPATH)/bin/gocyclo -over 40 $(DIR);)

deadcode:
	@echo "Running $@:"
	@$(GOPATH)/bin/deadcode

build: getdeps verifiers

test: getdeps verifiers
	@echo "Running all testing:"
	@$(foreach DIR, $(DIRS), go test $(GOFLAGS) $(DIR);)

gomake-all: build
	@echo "Installing minfs:"
	@go build --ldflags "$(LDFLAGS)" github.com/minio/minfs

coverage: getdeps verifiers
	@echo "Running all coverage:"
	@./buildscripts/go-coverage.sh

pkg-validate-arg-%: ;
ifndef PKG
	$(error Usage: make $(@:pkg-validate-arg-%=pkg-%) PKG=pkg_name)
endif

pkg-add: pkg-validate-arg-add
	@$(GOPATH)/bin/govendor add $(PKG)

pkg-update: pkg-validate-arg-update
	@$(GOPATH)/bin/govendor update $(PKG)

pkg-remove: pkg-validate-arg-remove
	@$(GOPATH)/bin/govendor remove $(PKG)

pkg-list:
	@$(GOPATH)/bin/govendor list

install: gomake-all
	@sudo /usr/bin/install -m 755 minfs /sbin/minfs && echo "Installing minfs"
	@sudo /usr/bin/install -m 755 mount.minfs /sbin/mount.minfs && echo "Installing mount.minfs"
	@sudo /usr/bin/install -m 644 docs/minfs.8 /usr/share/man/man8/minfs.8 && echo "Installing minfs.8"
	@sudo /usr/bin/install -m 644 docs/mount.minfs.8 /usr/share/man/man8/mount.minfs.8 && echo "Installing mount.minfs.8"

release: verifiers
	@MINFS_RELEASE=RELEASE ./buildscripts/build.sh

experimental: verifiers
	@MINFS_RELEASE=EXPERIMENTAL ./buildscripts/build.sh

clean:
	@rm -f cover.out
	@rm -f minfs
	@find . -name '*.test' | xargs rm -fv
	@rm -fr release
