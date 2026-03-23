.PHONY: build run test clean generate

BINARY=websessions
BUILD_DIR=bin

build: generate
	go build -o $(BUILD_DIR)/$(BINARY) ./cmd/websessions

run: generate
	go run ./cmd/websessions

test:
	go test ./... -v

clean:
	rm -rf $(BUILD_DIR)

generate:
	templ generate

lint:
	golangci-lint run ./...
