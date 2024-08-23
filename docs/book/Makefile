MDBOOK_VERSION := v0.4.40
TOOLS_DIR := $(realpath ../../hack/tools)
BIN_DIR := bin
TOOLS_BIN_DIR := $(TOOLS_DIR)/$(BIN_DIR)
MDBOOK_INSTALL := $(realpath ../../scripts/install-mdbook.sh)
EMBED := $(TOOLS_BIN_DIR)/mdbook-embed
MDBOOK := $(TOOLS_BIN_DIR)/mdbook

$(TOOLS_BIN_DIR)/%:
	make -C $(TOOLS_DIR) $(subst $(TOOLS_DIR)/,,$@)

$(MDBOOK):
	$(MDBOOK_INSTALL) $(MDBOOK_VERSION) $(TOOLS_BIN_DIR)

BOOK_DEPS := $(MDBOOK) $(EMBED)

.PHONY: serve
serve: $(BOOK_DEPS) ## Run a local web server with the compiled book
	$(MDBOOK) serve

.PHONY: build
build: $(BOOK_DEPS) ## Build the book
	$(MDBOOK) build

.PHONY: clean
clean:
	rm -rf book