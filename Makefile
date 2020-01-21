SHELL             = bash -o pipefail
TEST_FLAGS        ?= -v

#MOD_VENDOR        ?= -mod=vendor
GOMODULES         ?= on

GITTAG 						?= $(shell git describe --exact-match --tags HEAD 2>/dev/null || :)
GITBRANCH 				?= $(shell git rev-parse --abbrev-ref HEAD 2>/dev/null || :)
LONGVERSION 			?= $(shell git describe --tags --long --abbrev=8 --always HEAD)$(echo -$GITBRANCH | tr / - | grep -v '\-master' || :)
VERSION 					?= $(if $(GITTAG),$(GITTAG),$(LONGVERSION))
GITCOMMIT 				?= $(shell git log -1 --date=iso --pretty=format:%H)
GITCOMMITDATE 		?= $(shell git log -1 --date=iso --pretty=format:%cd)


all:
	@echo "***********************************"
	@echo "**         ethkit build          **"
	@echo "***********************************"
	@echo "make <cmd>"
	@echo ""
	@echo "commands:"
	@echo ""
	@echo " + Development:"
	@echo "   - build"
	@echo "   - test"
	@echo ""
	@echo ""
	@echo " + Dep management:"
	@echo "   - dep-upgrade-all"
	@echo ""


build: build-pkgs build-cli

build-pkgs:
	go build ./...

build-cli:
	@GOBIN=$$PWD/bin $(MAKE) install

install:
	GOGC=off GO111MODULE=$(GOMODULES)  \
	go install -v \
		$(MOD_VENDOR) \
		-ldflags='-X "main.VERSION=$(VERSION)" -X "main.GITBRANCH=$(GITBRANCH)" -X "main.GITCOMMIT=$(GITCOMMIT)" -X "main.GITCOMMITDATE=$(GITCOMMITDATE)"' \
		./cmd/ethkit

clean:
	rm -rf ./bin

test:
	GOGC=off GO111MODULE=$(GOMODULES) go test $(TEST_FLAGS) $(MOD_VENDOR) -run=$(TEST) ./...

test-clean:
	GOGC=off GO111MODULE=$(GOMODULES) go clean -testcache

dep-upgrade-all:
	GO111MODULE=on go get -u ./...
