BINARY_NAME=ed-assist
CMD_PATH=./cmd/ed-assist
WEB_BINARY_NAME=ed-assist-web
WEB_CMD_PATH=./cmd/ed-assist-web
OUTPUT_DIR=bin
VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS=-s -w -X main.Version=$(VERSION)

.PHONY: all build build-web windows windows-amd64 windows-arm64 linux test clean help

all: test build build-web windows

## build: Build for current OS and architecture
build:
	@mkdir -p $(OUTPUT_DIR)
	go build -ldflags="$(LDFLAGS)" -o $(OUTPUT_DIR)/$(BINARY_NAME) $(CMD_PATH)
	@echo "Built $(OUTPUT_DIR)/$(BINARY_NAME)"

## build-web: Build ed-assist-web for current OS and architecture
build-web:
	@mkdir -p $(OUTPUT_DIR)
	go build -ldflags="$(LDFLAGS)" -o $(OUTPUT_DIR)/$(WEB_BINARY_NAME) $(WEB_CMD_PATH)
	@echo "Built $(OUTPUT_DIR)/$(WEB_BINARY_NAME)"

## windows: Cross-compile for Windows (x86_64 / amd64)
windows: windows-amd64

## windows-amd64: Cross-compile for Windows 64-bit (x86_64)
windows-amd64:
	@mkdir -p $(OUTPUT_DIR)
	GOOS=windows GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o $(OUTPUT_DIR)/$(BINARY_NAME).exe $(CMD_PATH)
	GOOS=windows GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o $(OUTPUT_DIR)/$(WEB_BINARY_NAME).exe $(WEB_CMD_PATH)
	@echo "Built Windows binaries: $(OUTPUT_DIR)/$(BINARY_NAME).exe and $(OUTPUT_DIR)/$(WEB_BINARY_NAME).exe"

## windows-arm64: Cross-compile for Windows ARM64
windows-arm64:
	@mkdir -p $(OUTPUT_DIR)
	GOOS=windows GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o $(OUTPUT_DIR)/$(BINARY_NAME)-arm64.exe $(CMD_PATH)
	GOOS=windows GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o $(OUTPUT_DIR)/$(WEB_BINARY_NAME)-arm64.exe $(WEB_CMD_PATH)
	@echo "Built Windows ARM64 binaries"

## linux: Build for Linux 64-bit (amd64)
linux:
	@mkdir -p $(OUTPUT_DIR)
	GOOS=linux GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o $(OUTPUT_DIR)/$(BINARY_NAME)-linux-amd64 $(CMD_PATH)
	GOOS=linux GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o $(OUTPUT_DIR)/$(WEB_BINARY_NAME)-linux-amd64 $(WEB_CMD_PATH)
	@echo "Built Linux binaries"

## test: Run unit tests
test:
	go test -v ./...

## clean: Remove build artifacts
clean:
	rm -rf $(OUTPUT_DIR)
	@echo "Cleaned build artifacts"

## help: Display this help screen
help:
	@echo "Usage: make <target>"
	@echo ""
	@echo "Targets:"
	@grep -E '^## ' $(MAKEFILE_LIST) | sed -e 's/## //g' | awk 'BEGIN {FS = ": "}; {printf "  %-18s %s\n", $$1, $$2}'
