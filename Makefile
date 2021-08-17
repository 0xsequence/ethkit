SHELL             = bash -o pipefail
TEST_FLAGS        ?= -v

MOD_VENDOR        ?= -mod=vendor
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

start-test-chain:
	cd ./tools/test-chain && yarn start:server

start-test-chain-detached:
	cd ./tools/test-chain && yarn start:server:detached

stop-test-chain-detached:
	cd ./tools/test-chain && yarn start:stop:detached

test-chain-logs:
	cd ./tools/test-chain && yarn chain:logs

clean:
	rm -rf ./bin

test: check-test-chain-running go-test

go-test:
	GOGC=off GO111MODULE=$(GOMODULES) go test $(TEST_FLAGS) $(MOD_VENDOR) -run=$(TEST) ./...

test-skip-reorgme:
	SKIP_REORGME=true GOGC=off GO111MODULE=$(GOMODULES) go test $(TEST_FLAGS) $(MOD_VENDOR) -run=$(TEST) ./...

test-clean:
	GOGC=off GO111MODULE=$(GOMODULES) go clean -testcache

.PHONY: vendor
vendor:
	@export GO111MODULE=on && \
		go mod tidy && \
		rm -rf ./vendor && \
		go mod vendor && \
		go run github.com/goware/modvendor -copy="**/*.c **/*.h **/*.s **/*.proto"

dep-upgrade-all:
	GO111MODULE=on go get -u ./...

check-test-chain-running:
	cd ./tools/test-chain && bash isRunning.sh
