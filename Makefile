BINARY  := teleport
BIN_DIR := ./bin
INSTALL := $(HOME)/.local/bin
CMD_PATH := .

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
PKG     := github.com/pascualchavez/teleport/internal/version
LDFLAGS := -X $(PKG).Version=$(VERSION) -X $(PKG).Commit=$(COMMIT) -X $(PKG).Date=$(DATE)

.PHONY: build build_release install uninstall clean

build:
	@echo "Building binary..."
	@mkdir -p $(BIN_DIR)
	@go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY) $(CMD_PATH)
	@echo "Binary created at $(BIN_DIR)/$(BINARY)"
	@echo "Installing to $(INSTALL)/$(BINARY)..."
	@cp -f $(BIN_DIR)/$(BINARY) "$(INSTALL)/$(BINARY)"
	@echo "Done — $(INSTALL)/$(BINARY) updated"

build_release:
	@echo "Building release binaries..."
	@rm -rf $(BIN_DIR)
	@mkdir -p $(BIN_DIR)
	@go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY)_darwin_arm64 $(CMD_PATH)
	@GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY)_darwin_amd64 $(CMD_PATH)
	@GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY)_linux_amd64 $(CMD_PATH)
	@GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY)_linux_arm64 $(CMD_PATH)
	@echo "Release binaries created in $(BIN_DIR)/"

install: build

uninstall:
	@rm -f $(INSTALL)/$(BINARY)
	@echo "Removed $(INSTALL)/$(BINARY)"

clean:
	@rm -rf $(BIN_DIR)
