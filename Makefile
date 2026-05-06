BINARY  := teleport
INSTALL := $(HOME)/.local/bin

.PHONY: build install uninstall clean

build:
	go build -o $(BINARY) .

install: build
	mkdir -p $(INSTALL)
	cp $(BINARY) $(INSTALL)/$(BINARY)
	@echo "Installed to $(INSTALL)/$(BINARY)"

uninstall:
	rm -f $(INSTALL)/$(BINARY)
	@echo "Removed $(INSTALL)/$(BINARY)"

clean:
	rm -f $(BINARY)
