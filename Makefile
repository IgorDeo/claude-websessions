.PHONY: build build-gui run run-gui test clean generate

BINARY=websessions
BUILD_DIR=bin

build: generate
	go build -o $(BUILD_DIR)/$(BINARY) ./cmd/websessions

build-gui: generate
	CGO_ENABLED=1 go build -tags gui -o $(BUILD_DIR)/$(BINARY) ./cmd/websessions

run: generate
	go run ./cmd/websessions

run-gui: generate
	CGO_ENABLED=1 go run -tags gui ./cmd/websessions --gui

test:
	go test ./... -v

clean:
	rm -rf $(BUILD_DIR)

generate:
	templ generate

lint:
	golangci-lint run ./...
