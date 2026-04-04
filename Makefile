LIBEXEC_DIR = $(HOME)/.local/libexec/sidings
BIN_DIR     = $(HOME)/.local/bin

.PHONY: build install uninstall dev clean test

build:
	go build -o bin/ ./cmd/...

install: build
	mkdir -p $(LIBEXEC_DIR) $(BIN_DIR)
	cp bin/task-classify   $(LIBEXEC_DIR)/
	cp bin/task-route      $(LIBEXEC_DIR)/
	cp bin/task-dispatch   $(LIBEXEC_DIR)/
	cp bin/task-decompose  $(LIBEXEC_DIR)/
	cp bin/task-merge      $(LIBEXEC_DIR)/
	cp bin/sidings         $(BIN_DIR)/
	@echo "installed. run: sidings --help"

uninstall:
	rm -rf $(LIBEXEC_DIR)
	rm -f  $(BIN_DIR)/sidings

# Convenience — rebuild and reinstall in one step
dev: install

clean:
	rm -rf bin/

test:
	go test ./...
