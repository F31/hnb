BINARY=network-provider
MODULE=github.com/F31/hnb/cmd/network-provider
BUILD_DIR=.
DOCKER_IMAGE=hnb/network-provider

.PHONY: all build test clean docker

all: build

build:
	go build -o $(BINARY) $(BUILD_DIR)

test:
	go test ./...

vet:
	go vet ./...

clean:
	rm -f $(BINARY)

docker:
	docker build -t $(DOCKER_IMAGE) .

.PHONY: lint
lint:
	golangci-lint run ./...