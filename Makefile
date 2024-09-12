EXECUTABLE=KIEMPossible
LINUX=$(EXECUTABLE)
DARWIN=$(EXECUTABLE)_darwin
VERSION=$(shell git describe --tags --always --long --dirty)

linux: $(LINUX) 

darwin: $(DARWIN) 

$(LINUX):
	env GOOS=linux GOARCH=amd64 go build -v -o ./bin/$(LINUX) -ldflags="-s -w -X main.version=$(VERSION)"  ./cmd/kiempossible/*.go

$(DARWIN):
	env GOOS=darwin GOARCH=amd64 go build -v -o ./bin/$(DARWIN) -ldflags="-s -w -X main.version=$(VERSION)"  ./cmd/kiempossible/*.go

build: linux darwin 
	@echo version: $(VERSION)

clean: 
	rm -f ./bin/$(LINUX)*

.PHONY: clean

help: 
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'
