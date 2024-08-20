# Directories.
BIN_DIR := bin

MDBOOK_EMBED := $(BIN_DIR)/mdbook-embed
$(MDBOOK_EMBED): $(BIN_DIR) go.mod go.sum
	go build -tags=tools -o $(BIN_DIR)/mdbook-embed sigs.k8s.io/cluster-api/hack/tools/mdbook/embed

