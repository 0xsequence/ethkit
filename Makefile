
all:
	@echo "Hello there. View the Makefile source for options"


##
## Tools
##
tools:
	GO111MODULE=off go get -u github.com/goware/modvendor


##
## Dependency mgmt
##
dep:
	@export GO111MODULE=on && \
		go mod tidy && \
		rm -rf ./vendor && go mod vendor && \
		modvendor -copy="**/*.c **/*.h **/*.s **/*.proto"

dep-upgrade-all:
	@GO111MODULE=on go get -u=patch
	@$(MAKE) dep
