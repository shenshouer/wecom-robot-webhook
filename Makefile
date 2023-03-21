BINARY_NAME := $(shell basename "$(PWD)")
GOBASE := $(shell pwd)
GOBIN := $(GOBASE)/bin

VERSION := $(shell git describe --tags --always)

GOOS := "linux"
GOARCH := "amd64"

REGISTRY := "docker.io"
IMAGE_NAME := shenshouer/$(BINARY_NAME)

all: build

build:
	@echo "  >  Building binary..."
	@GOOS=$(GOOS) GOARCH=$(GOARCH) go build -o $(GOBIN)/$(BINARY_NAME) main.go

image:
	@echo "  >  Building image..."
	@docker build -t $(REGISTRY)/$(IMAGE_NAME):$(VERSION) .

push:
	@echo "  >  Pushing image..."
	@docker push $(REGISTRY)/$(IMAGE_NAME):$(VERSION)