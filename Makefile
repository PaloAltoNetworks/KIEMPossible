EXECUTABLE=KIEMScanner
LINUX=$(EXECUTABLE)
DARWIN=$(EXECUTABLE)_darwin_amd64
VERSION=$(shell git describe --tags --always --long --dirty)

linux: $(LINUX) ## Build for Linux

darwin: $(DARWIN) ## Build for Darwin (macOS)

$(LINUX):
	env GOOS=linux GOARCH=amd64 go build -v -o ./bin/$(LINUX) -ldflags="-s -w -X main.version=$(VERSION)"  ./cmd/kiemscanner/*.go

$(DARWIN):
	env GOOS=darwin GOARCH=amd64 go build -v -o ./bin/$(DARWIN) -ldflags="-s -w -X main.version=$(VERSION)"  ./cmd/kiemscanner/*.go

build: linux darwin ## Build binaries
	@echo version: $(VERSION)

all: test build ## Build and run tests

test: ## Run unit tests
	go test ./...

clean: ## Remove previous build
	rm -f $(LINUX) $(DARWIN)

.PHONY: all test clean

help: ## Display available commands
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'
