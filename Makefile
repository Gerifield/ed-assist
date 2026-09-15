BINARY_NAME=ed-assist
CMD_PATH=./cmd/ed-assist
OUTPUT_DIR=bin
LDFLAGS=-s -w

.PHONY: all build windows windows-amd64 windows-arm64 linux test clean help

all: test build windows

## build: Build for current OS and architecture
build:
	@mkdir -p $(OUTPUT_DIR)
	go build -ldflags="$(LDFLAGS)" -o $(OUTPUT_DIR)/$(BINARY_NAME) $(CMD_PATH)
	@echo "Built $(OUTPUT_DIR)/$(BINARY_NAME)"

## windows: Cross-compile for Windows (x86_64 / amd64)
windows: windows-amd64

## windows-amd64: Cross-compile for Windows 64-bit (x86_64)
windows-amd64:
	@mkdir -p $(OUTPUT_DIR)
	GOOS=windows GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o $(OUTPUT_DIR)/$(BINARY_NAME).exe $(CMD_PATH)
	@echo "Built Windows binary: $(OUTPUT_DIR)/$(BINARY_NAME).exe"

## windows-arm64: Cross-compile for Windows ARM64
windows-arm64:
	@mkdir -p $(OUTPUT_DIR)
	GOOS=windows GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o $(OUTPUT_DIR)/$(BINARY_NAME)-arm64.exe $(CMD_PATH)
	@echo "Built Windows ARM64 binary: $(OUTPUT_DIR)/$(BINARY_NAME)-arm64.exe"

## linux: Build for Linux 64-bit (amd64)
linux:
	@mkdir -p $(OUTPUT_DIR)
	GOOS=linux GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o $(OUTPUT_DIR)/$(BINARY_NAME)-linux-amd64 $(CMD_PATH)
	@echo "Built Linux binary: $(OUTPUT_DIR)/$(BINARY_NAME)-linux-amd64"

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
