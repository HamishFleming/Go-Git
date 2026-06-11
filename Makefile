APP := go-git
CMD := ./cmd/$(APP)
BUILD_DIR := ./bin
BUILD_BIN := $(BUILD_DIR)/$(APP)
INSTALL_DIR := $(HOME)/.bin
INSTALL_BIN := $(INSTALL_DIR)/$(APP)

.PHONY: build install clean

build:
	mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_BIN) $(CMD)

install: build
	mkdir -p $(INSTALL_DIR)
	cp $(BUILD_BIN) $(INSTALL_BIN)

clean:
	rm -f $(BUILD_BIN)
