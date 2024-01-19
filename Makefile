BINARY_NAME := $(shell basename "$(PWD)")
GOBASE := $(shell pwd)
GOBIN := $(GOBASE)/bin

#VERSION := $(shell git describe --tags --always)
VERSION := v0.8

GOOS := "linux"
GOARCH := "amd64"

REGISTRY := "registry.eeo-inc.com"
IMAGE_NAME := devops/$(BINARY_NAME)

all: build

build:
	@echo "  >  Building binary..."
	@GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=0 go build -o $(GOBIN)/$(BINARY_NAME) main.go

image:
	@echo "  >  Building image..."
	@docker build --platform linux/amd64 -t $(REGISTRY)/$(IMAGE_NAME):$(VERSION) .

push:
	@echo "  >  Pushing image..."
	@docker push $(REGISTRY)/$(IMAGE_NAME):$(VERSION)