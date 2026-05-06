BINARY  := teleport
INSTALL := $(HOME)/.local/bin

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
PKG     := github.com/pascualchavez/teleport/internal/version
LDFLAGS := -X $(PKG).Version=$(VERSION) -X $(PKG).Commit=$(COMMIT) -X $(PKG).Date=$(DATE)

.PHONY: build install uninstall clean

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) .

install: build
	mkdir -p $(INSTALL)
	cp $(BINARY) $(INSTALL)/$(BINARY)
	@echo "Installed to $(INSTALL)/$(BINARY)"

uninstall:
	rm -f $(INSTALL)/$(BINARY)
	@echo "Removed $(INSTALL)/$(BINARY)"

clean:
	rm -f $(BINARY)
