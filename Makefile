#
# Based on http://chrismckenzie.io/post/deploying-with-golang/
#          https://raw.githubusercontent.com/lpabon/quartermaster/dev/Makefile
#

.PHONY: version all operator run clean container deploy talisman

DOCKER_HUB_TAG ?= latest
DOCKER_PULLER_IMG=$(DOCKER_HUB_REPO)/docker-puller:$(DOCKER_HUB_TAG)
PX_NODE_WIPER_IMG=$(DOCKER_HUB_REPO)/px-node-wiper:$(DOCKER_HUB_TAG)
TALISMAN_IMG=$(DOCKER_HUB_REPO)/talisman:$(DOCKER_HUB_TAG)

SHA := $(shell git rev-parse --short HEAD)
BRANCH := $(subst /,-,$(shell git rev-parse --abbrev-ref HEAD))
VER := $(shell git rev-parse --short HEAD)
ARCH := $(shell go env GOARCH)
GOOS := $(shell go env GOOS)
DIR=.

ifndef TAGS
TAGS := daemon
endif

ifdef APP_SUFFIX
  VERSION = $(VER)-$(subst /,-,$(APP_SUFFIX))
else
ifeq (master,$(BRANCH))
  VERSION = $(VER)
else
  VERSION = $(VER)-$(BRANCH)
endif
endif

# Go setup
GO=go

# Sources and Targets
# Build Binaries setting main.version and main.build vars
LDFLAGS :=-ldflags "-X github.com/portworx/talisman/pkg/version.Version=$(VERSION) -extldflags '-z relro -z now'"
PKGS=$(shell go list ./... | grep -v vendor)
GOVET_PKGS=$(shell  go list ./... | grep -v vendor | grep -v pkg/client/informers/externalversions | grep -v versioned)

BASE_DIR := $(shell git rev-parse --show-toplevel)

BIN :=$(BASE_DIR)/bin
GOFMT := gofmt

.DEFAULT: all

all: clean pretest operator talisman

# print the version
version:
	@echo $(VERSION)

operator: codegen
	mkdir -p $(BIN)
	go build $(LDFLAGS) -o $(BIN)/operator cmd/operator/main.go

talisman: codegen
	mkdir -p $(BIN)
	go build $(LDFLAGS) -o $(BIN)/talisman cmd/talisman/talisman.go

test:
	go test -tags "$(TAGS)" $(TESTFLAGS) $(PKGS)

fmt:
	@echo "Performing gofmt on following: $(PKGS)"
	@./hack/do-gofmt.sh $(PKGS)

checkfmt:
	@echo "Checking gofmt on following: $(PKGS)"
	@./hack/check-gofmt.sh $(PKGS)

lint:
	@echo "golint"
	go get -v github.com/golang/lint/golint
	for file in $$(find . -name '*.go' | grep -v vendor | \
																			grep -v '\.pb\.go' | \
																			grep -v '\.pb\.gw\.go' | \
																			grep -v 'externalversions' | \
																			grep -v 'versioned' | \
																			grep -v 'generated'); do \
		golint $${file}; \
		if [ -n "$$(golint $${file})" ]; then \
			exit 1; \
		fi; \
	done

vet:
	@echo "govet"
	go vet $(GOVET_PKGS)

errcheck:
	@echo "errcheck"
	go get -v github.com/kisielk/errcheck
	errcheck -tags "$(TAGS)" $(GOVET_PKGS)

codegen:
	@echo "Generating files"
	@./hack/update-codegen.sh

verifycodegen:
	@echo "Verifying generated files"
	@./hack/verify-codegen.sh

pretest: checkfmt vet lint errcheck verifycodegen

container:
	sudo docker build --tag $(TALISMAN_IMG) -f Dockerfile.talisman .
	sudo docker build --tag $(DOCKER_PULLER_IMG) cmd/docker-puller/
	sudo docker build -t $(PX_NODE_WIPER_IMG) cmd/px-node-wiper/

deploy: container
	sudo docker push $(TALISMAN_IMG)
	sudo docker push $(DOCKER_PULLER_IMG)
	sudo docker push $(PX_NODE_WIPER_IMG)

docker-build:
	docker build -t px/docker-build -f Dockerfile.build .
	@echo "Building using docker"
	docker run \
		--privileged \
		-v $(shell pwd):/go/src/github.com/portworx/talisman \
		px/docker-build make all

.PHONY: test clean name run version

clean:
	@echo Cleaning Workspace...
	-sudo rm -rf $(BIN)
	-docker rmi -f $(OPERATOR_IMG)
	-docker rmi -f $(DOCKER_PULLER_IMG)
	-docker rmi -f $(PX_NODE_WIPER_IMG)
	-docker rmi -f $(TALISMAN_IMG)
	go clean -i $(PKGS)
