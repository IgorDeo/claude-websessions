.PHONY: build build-gui run run-gui test clean generate

BINARY=websessions
BUILD_DIR=bin
HELPER=webview-helper

build: generate
	go build -o $(BUILD_DIR)/$(BINARY) ./cmd/websessions

build-gui: generate build-webview-helper
	go build -tags gui -o $(BUILD_DIR)/$(BINARY) ./cmd/websessions

build-webview-helper:
	cc $$(pkg-config --cflags gtk+-3.0 webkit2gtk-4.1) \
		cmd/webview-helper/main.c \
		$$(pkg-config --libs gtk+-3.0 webkit2gtk-4.1) \
		-o cmd/websessions/$(HELPER)

run: generate
	go run ./cmd/websessions

run-gui: generate build-webview-helper
	go run -tags gui ./cmd/websessions --gui

test:
	go test ./... -v

clean:
	rm -rf $(BUILD_DIR) cmd/websessions/$(HELPER)

generate:
	templ generate

lint:
	golangci-lint run ./...
