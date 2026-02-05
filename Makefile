.PHONY: build build-all clean test

VERSION := 2.0.0
LDFLAGS := -ldflags "-s -w"
BINARY := codeanalyzer-go
BIN_DIR := bin

# Build for current platform
build:
	go build $(LDFLAGS) -o $(BIN_DIR)/$(BINARY) ./cmd/codeanalyzer-go

# Build for all platforms (64-bit only)
build-all: clean
	@echo "Building for all platforms..."
	@mkdir -p $(BIN_DIR)
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(BIN_DIR)/$(BINARY)-windows-amd64.exe ./cmd/codeanalyzer-go
	GOOS=linux   GOARCH=amd64 go build $(LDFLAGS) -o $(BIN_DIR)/$(BINARY)-linux-amd64 ./cmd/codeanalyzer-go
	GOOS=darwin  GOARCH=amd64 go build $(LDFLAGS) -o $(BIN_DIR)/$(BINARY)-darwin-amd64 ./cmd/codeanalyzer-go
	GOOS=darwin  GOARCH=arm64 go build $(LDFLAGS) -o $(BIN_DIR)/$(BINARY)-darwin-arm64 ./cmd/codeanalyzer-go
	@echo "Done! Binaries in $(BIN_DIR)/"
	@ls -lh $(BIN_DIR)/

# Clean build artifacts
clean:
	rm -rf $(BIN_DIR)/*

# Run tests
test:
	go test -v ./...

# Run integration tests
test-integration: build
	python tests/cldk_integration_test.py
