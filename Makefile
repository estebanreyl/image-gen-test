BINARY_PATH=./bin/acr
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
MAIN_FOLDER=./acr/

clean:
	rm -f $(BINARY_PATH)

build:
	$(GOBUILD) -ldflags="-X main.Version=`git log --format="%h" -n 1`" -o $(BINARY_PATH) $(MAIN_FOLDER)

test:
	$(GOTEST) -v -vet=off $(MAIN_FOLDER)

all: clean test build

.DEFAULT_GOAL := all
.PHONY: all clean build test
