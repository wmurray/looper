BIN := looper
INSTALL_DIR := $(HOME)/.local/bin

.PHONY: build install uninstall

build:
	go build -o $(BIN) .

install: build
	mv $(BIN) $(INSTALL_DIR)/$(BIN)
	@echo "Installed to $(INSTALL_DIR)/$(BIN)"

uninstall:
	rm -f $(INSTALL_DIR)/$(BIN)
	@echo "Removed $(INSTALL_DIR)/$(BIN)"
