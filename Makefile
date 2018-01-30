#
# Based on http://chrismckenzie.io/post/deploying-with-golang/
#          https://raw.githubusercontent.com/lpabon/quartermaster/dev/Makefile
#

.PHONY: version all operator run clean container deploy

OPERATOR_IMG=$(DOCKER_HUB_REPO)/$(DOCKER_HUB_OPERATOR_IMAGE):$(DOCKER_HUB_TAG)

APP_NAME := operator
SHA := $(shell git rev-parse --short HEAD)
BRANCH := $(subst /,-,$(shell git rev-parse --abbrev-ref HEAD))
VER := $(shell git rev-parse --short HEAD)
ARCH := $(shell go env GOARCH)
GOOS := $(shell go env GOOS)
+GLIDEPATH := $(shell command -v glide 2> /dev/null)
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
EXECUTABLES :=$(APP_NAME)
# Build Binaries setting main.version and main.build vars
LDFLAGS :=-ldflags "-X main.PX_OPERATOR_VERSION=$(VERSION) -extldflags '-z relro -z now'"
PKGS=$(shell go list ./... | grep -v vendor)
GOVET_PKGS=$(shell  go list ./... | grep -v vendor | grep -v pkg/client/informers/externalversions | grep -v versioned)

BASE_DIR := $(shell git rev-parse --show-toplevel)

BIN :=$(BASE_DIR)/bin
GOFMT := gofmt

.DEFAULT: all

all: clean pretest operator

# print the version
version:
	@echo $(VERSION)

# print the name of the app
name:
	@echo $(APP_NAME)

# print the package path
package:
	@echo $(PACKAGE)

operator: codegen
	mkdir -p $(BIN)
	go build $(LDFLAGS) -o $(BIN)/$(APP_NAME) cmd/operator/main.go

run: operator
	$(BIN)/$(APP_NAME)

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
	@echo "Building container: docker build --tag $(OPERATOR_IMG) -f Dockerfile ."
	sudo docker build --tag $(OPERATOR_IMG) -f Dockerfile .

deploy: container
	sudo docker push $(OPERATOR_IMG)

docker-build:
	docker build -t px/docker-build -f Dockerfile.build .
	@echo "Building px operator using docker"
	docker run \
		--privileged \
		-v $(shell pwd):/go/src/github.com/portworx/talisman \
		-e DOCKER_HUB_REPO=$(DOCKER_HUB_REPO) \
		-e DOCKER_HUB_TORPEDO_IMAGE=$(DOCKER_HUB_TORPEDO_IMAGE) \
		-e DOCKER_HUB_TAG=$(DOCKER_HUB_TAG) \
		px/docker-build make all

.PHONY: test clean name run version

clean:
	@echo Cleaning Workspace...
	-sudo rm -rf $(BIN)
	@echo "Deleting image "$(OPERATOR_IMG)
	-docker rmi -f $(OPERATOR_IMG)
	go clean -i $(PKGS)
