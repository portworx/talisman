#
# Based on http://chrismckenzie.io/post/deploying-with-golang/
#          https://raw.githubusercontent.com/lpabon/quartermaster/dev/Makefile
#

.PHONY: version all run clean container deploy talisman

DOCKER_HUB_TAG ?= latest
PX_NODE_WIPER_TAG ?= latest
DOCKER_PULLER_IMG=$(DOCKER_HUB_REPO)/docker-puller:$(DOCKER_HUB_TAG)
PX_NODE_WIPER_IMG=$(DOCKER_HUB_REPO)/px-node-wiper:$(PX_NODE_WIPER_TAG)
TALISMAN_IMG=$(DOCKER_HUB_REPO)/talisman:$(DOCKER_HUB_TAG)

SHA    := $(shell git rev-parse --short HEAD)
BRANCH := $(subst /,-,$(shell git rev-parse --abbrev-ref HEAD))
VER    := $(shell git rev-parse --short HEAD)
GO     := go
GOENV  := GOOS=linux GOARCH=amd64
DIR=.

DOCK_BUILD_CNT	:= golang:1.21

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

LDFLAGS += -X github.com/portworx/talisman/pkg/version.Version=$(VERSION)

BUILD_TYPE=static
ifeq ($(BUILD_TYPE),static)
    LDFLAGS += -extldflags -static
    BUILD_OPTIONS += -v -a -ldflags "$(LDFLAGS)"
    GOENV += CGO_ENABLED=0
else ifeq ($(BUILD_TYPE),debug)
    BUILD_OPTIONS += -i -v -gcflags "-N -l" -ldflags "$(LDFLAGS)"
else
    BUILD_OPTIONS += -i -v -ldflags "$(LDFLAGS)"
endif

# Talisman can only be built with go 1.11+ which supports go modules
export GO111MODULE=on
export GOFLAGS = -mod=vendor

PKGS=$(shell go list ./... | grep -v vendor)
GOVET_PKGS=$(shell  go list ./... | grep -v vendor | grep -v pkg/client/informers/externalversions | grep -v versioned)

BASE_DIR := $(shell git rev-parse --show-toplevel)

BIN :=$(BASE_DIR)/bin
GOFMT := gofmt

.DEFAULT: all

vendor-tidy:
	go mod tidy

vendor-update:
	go mod download

vendor:
	go mod vendor

all: clean pretest talisman

# print the version
version:
	@echo $(VERSION)

talisman:
	mkdir -p $(BIN)
	env $(GOENV) go build $(BUILD_OPTIONS) -o $(BIN)/talisman cmd/talisman/talisman.go

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
	go install golang.org/x/lint/golint
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
	go install github.com/kisielk/errcheck
	errcheck -tags "$(TAGS)" $(GOVET_PKGS)

codegen:
	@echo "Generating files"
	@./hack/update-codegen.sh

verifycodegen:
	@echo "Verifying generated files"
	@./hack/verify-codegen.sh

pretest: checkfmt vet lint errcheck

container:
	sudo docker build --pull --tag $(TALISMAN_IMG) -f Dockerfile.talisman .
	sudo docker build --pull --tag $(PX_NODE_WIPER_IMG) cmd/px-node-wiper/

deploy: container
	sudo docker push $(TALISMAN_IMG)
	sudo docker push $(PX_NODE_WIPER_IMG)

docker-build:
	@echo "Building using docker (docker-build)"
	docker run --privileged=true --rm -v $(shell pwd):/go/src/github.com/portworx/talisman -v $(HOME)/.gitconfig:/root/.gitconfig $(DOCK_BUILD_CNT) \
		/bin/bash -c "cd /go/src/github.com/portworx/talisman; make all"


.PHONY: test clean name run version

container-clean:
	-docker rmi -f $(PX_NODE_WIPER_IMG)
	-docker rmi -f $(TALISMAN_IMG)

clean:
	@echo Cleaning Workspace...
	-[ $(shell id -u) -ne 0 ] && sudo rm -rf $(BIN) || rm -rf $(BIN)
	go clean -i $(PKGS)
