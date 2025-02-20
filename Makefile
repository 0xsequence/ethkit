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

clean:
	rm -rf ./bin

install:
	GOGC=off GO111MODULE=$(GOMODULES)  \
	go install -v \
		$(MOD_VENDOR) \
		-ldflags='-X "main.VERSION=$(VERSION)" -X "main.GITBRANCH=$(GITBRANCH)" -X "main.GITCOMMIT=$(GITCOMMIT)" -X "main.GITCOMMITDATE=$(GITCOMMITDATE)"' \
		./cmd/ethkit


#
# Testing
#

# Run baseline tests
test: check-testchain-running go-test

# Run testchain and tests concurrently
test-concurrently:
	cd ./tools/testchain && pnpm concurrently -k --success first 'pnpm start:hardhat' 'cd ../.. && make go-test'

# Run tests with reorgme
test-with-reorgme: check-reorgme-running
	REORGME=true $(MAKE) go-test

# Go test short-hand, and skip testing go-ethereum
go-test: test-clean
	GOGC=off go test $(TEST_FLAGS) $(MOD_VENDOR) -race -run=$(TEST) `go list ./... | grep -v go-ethereum`

# Go test short-hand, including go-ethereum
go-test-all: test-clean
	GOGC=off go test $(TEST_FLAGS) $(MOD_VENDOR) -run=$(TEST) ./...

test-clean:
	GOGC=off go clean -testcache

.PHONY: tools
tools:
	cd ./ethtest/testchain && pnpm install
	cd ./ethtest/reorgme && pnpm install

bootstrap:
	cd ./ethtest/testchain && pnpm install


#
# Testchain
#
start-testchain:
	cd ./ethtest/testchain && pnpm start:hardhat

start-testchain-verbose:
	cd ./ethtest/testchain && pnpm start:hardhat:verbose

start-testchain-geth:
	cd ./ethtest/testchain && pnpm start:geth

start-testchain-geth-verbose:
	cd ./ethtest/testchain && pnpm start:geth:verbose

start-testchain-anvil:
	cd ./ethtest/testchain && pnpm start:anvil

start-testchain-anvil-verbose:
	cd ./ethtest/testchain && pnpm start:anvil:verbose

check-testchain-running:
	@curl http://localhost:8545 -H"Content-type: application/json" -X POST -d '{"jsonrpc":"2.0","method":"eth_syncing","params":[],"id":1}' --write-out '%{http_code}' --silent --output /dev/null | grep 200 > /dev/null \
	|| { echo "*****"; echo "Oops! testchain is not running. Please run 'make start-testchain' in another terminal or use 'test-concurrently'."; echo "*****"; exit 1; }


#
# Reorgme
#
start-reorgme:
	cd ./ethtest/reorgme && pnpm start:server

start-reorgme-detached:
	cd ./ethtest/reorgme && pnpm start:server:detached

stop-reorgme-detached:
	cd ./ethtest/reorgme && pnpm start:stop:detached

reorgme-logs:
	cd ./ethtest/reorgme && pnpm chain:logs

check-reorgme-running:
	cd ./ethtest/reorgme && bash isRunning.sh


#
# Dep management
#
dep-upgrade-all:
	GO111MODULE=on go get -u ./...

# .PHONY: vendor
# vendor:
# 	@export GO111MODULE=on && \
# 		go mod tidy && \
# 		rm -rf ./vendor && \
# 		go mod vendor && \
# 		go run github.com/goware/modvendor -copy="**/*.c **/*.h **/*.s **/*.proto"
