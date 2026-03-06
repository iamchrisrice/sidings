LIBEXEC_DIR = $(HOME)/.local/libexec/sidings
BIN_DIR     = $(HOME)/.local/bin

.PHONY: build install uninstall clean

build:
	go build -o bin/ ./cmd/sidings/
	go build -o bin/ ./cmd/internal/...

install: build
	mkdir -p $(LIBEXEC_DIR)
	cp bin/task-classify $(LIBEXEC_DIR)/
	cp bin/task-route $(LIBEXEC_DIR)/
	cp bin/task-dispatch $(LIBEXEC_DIR)/
	cp bin/monitor $(LIBEXEC_DIR)/
	cp bin/task-decompose $(LIBEXEC_DIR)/
	cp bin/task-merge $(LIBEXEC_DIR)/
	mkdir -p $(BIN_DIR)
	cp bin/sidings $(BIN_DIR)/
	@echo "installed. run: sidings --help"

uninstall:
	rm -rf $(LIBEXEC_DIR)
	rm -f $(BIN_DIR)/sidings

clean:
	rm -rf bin/

test:
	go test ./...
