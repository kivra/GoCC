# Variables
BINARY_NAME=gocc
GO=go
DOCKER_IMAGE_REPO ?= "somewhere.over/the/rainbow"
DOCKER_IMAGE_NAME ?= "gocc"
DOCKER_IMAGE_TAG ?= "latest"

# Targets
.PHONY: all build clean test run

all: build

build:
	$(GO) build -o $(BINARY_NAME) .

docker: build
	docker build -t $(BINARY_NAME) . -t $(DOCKER_IMAGE_REPO)/$(DOCKER_IMAGE_NAME):$(DOCKER_IMAGE_TAG)

release: build lint vet test docker

clean:
	$(GO) clean
	rm -f $(BINARY_NAME)

test:
	$(GO) test ./... -count=1

run:
	$(GO) run .

# Additional useful targets
.PHONY: deps lint fmt vet

deps:
	$(GO) mod download

lint:
	golangci-lint run

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

benchmark:
	$(GO) run . benchmark actors
