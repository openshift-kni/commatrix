FORMAT ?= csv
DEST_DIR ?= .
DEBUG ?= false
SUITE ?= all
GO_SRC := cmd/main.go
EXECUTABLE := oc-commatrix
export GOLANGCI_LINT_CACHE = /tmp/.cache

.DEFAULT_GOAL := run

SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec
CURPATH=$(PWD)
BIN_DIR=$(CURPATH)/bin
INSTALL_DIR = /usr/local/bin

GOLANGCI_LINT = $(BIN_DIR)/golangci-lint
# golangci-lint version should be updated periodically
# we keep it fixed to avoid it from unexpectedly failing on the project
# in case of a version bump
GOLANGCI_LINT_VER = v1.63.4

# Output directory
OUTPUT_DIR=$(CURPATH)/cross-build-output

# Supported platforms
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64

# Setting of GOOS and GOARCH for the build
GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)

GOOS := $(if $(GOOS),$(GOOS),linux)   
GOARCH := $(if $(GOARCH),$(GOARCH),amd64)

# Default goal
.DEFAULT_GOAL := build

# Build for current platform
.PHONY: build
build:
	GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=0 go build -o $(EXECUTABLE) $(GO_SRC)

# Build for all platforms
.PHONY: cross-build
cross-build:
	@mkdir -p $(OUTPUT_DIR)
	@for platform in $(PLATFORMS); do \
		GOOS=$$(echo $$platform | cut -d'/' -f1); \
		GOARCH=$$(echo $$platform | cut -d'/' -f2); \
		EXT=""; \
		if [ "$$GOOS" = "windows" ]; then EXT=".exe"; fi; \
		OUTPUT="$(OUTPUT_DIR)/$(EXECUTABLE)_$${GOOS}_$${GOARCH}$${EXT}"; \
		echo "Building for $$GOOS/$$GOARCH..."; \
		echo "Generating: $$OUTPUT"; \
		GOOS=$$GOOS GOARCH=$$GOARCH CGO_ENABLED=0 go build -o $$OUTPUT $(GO_SRC); \
	done

# Clean built executables
.PHONY: clean-cross-build
clean-cross-build:
	rm -rf $(OUTPUT_DIR) $(EXECUTABLE)


.PHONY: generate
generate: build
	rm -rf $(DEST_DIR)/communication-matrix
	mkdir -p $(DEST_DIR)/communication-matrix
	./$(EXECUTABLE) generate --format=$(FORMAT) --destDir=$(DEST_DIR)/communication-matrix --customEntriesPath=$(CUSTOM_ENTRIES_PATH) --customEntriesFormat=$(CUSTOM_ENTRIES_FORMAT) --host-open-ports $(if $(DEBUG),--debug=true)

.PHONY: install
install:
	install $(EXECUTABLE) $(INSTALL_DIR)


.PHONY: clean
clean:
	@rm -f $(EXECUTABLE)

mock-generate: gomock
	go generate ./...

# Run go fmt against code
fmt-code:
	go fmt ./...

GOMOCK = $(shell pwd)/bin/mockgen
gomock:
	$(call go-install-tool,$(GOMOCK),github.com/golang/mock/mockgen@v1.6.0)

GINKGO = $(BIN_DIR)/ginkgo
ginkgo:
	$(call go-install-tool,$(GINKGO),github.com/onsi/ginkgo/v2/ginkgo)

deps-update:
	go mod tidy

check-deps: deps-update
	@set +e; git diff --quiet HEAD go.sum go.mod; \
	if [ $$? -eq 1 ]; \
	then echo -e "\ngo modules are out of date. Please commit after running 'make deps-update' command\n"; \
	exit 1; fi

$(GOLANGCI_LINT): ; $(info installing golangci-lint...)
	$(call go-install-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/cmd/golangci-lint@$(GOLANGCI_LINT_VER))

.PHONY: lint
lint: | $(GOLANGCI_LINT) ; $(info  running golangci-lint...) @ ## Run golangci-lint
	GOFLAGS="" $(GOLANGCI_LINT) run --timeout=10m

.PHONY: test
test:
	GOFLAGS="" go test ./pkg/...
	GOFLAGS="" go test ./cmd/...


.PHONY: e2e-test
e2e-test: ginkgo
	@if [ "$(SUITE)" = "Validation" ] || [ "$(SUITE)" = "Nftables" ]; then \
		echo "Running e2e '$(SUITE)' test suite"; \
		EXTRA_NFTABLES_MASTER_FILE="$(EXTRA_NFTABLES_MASTER_FILE)" EXTRA_NFTABLES_WORKER_FILE="$(EXTRA_NFTABLES_WORKER_FILE)" \
		OPEN_PORTS_TO_IGNORE_IN_DOC_TEST_FILE="$(OPEN_PORTS_TO_IGNORE_IN_DOC_TEST_FILE)" OPEN_PORTS_TO_IGNORE_IN_DOC_TEST_FORMAT="$(OPEN_PORTS_TO_IGNORE_IN_DOC_TEST_FORMAT)" \
		$(GINKGO) -v --focus "$(SUITE)" ./test/e2e/...; \
	elif [ "$(SUITE)" = "all" ]; then \
		echo "Running all e2e test suites"; \
		EXTRA_NFTABLES_MASTER_FILE="$(EXTRA_NFTABLES_MASTER_FILE)" EXTRA_NFTABLES_WORKER_FILE="$(EXTRA_NFTABLES_WORKER_FILE)" \
		OPEN_PORTS_TO_IGNORE_IN_DOC_TEST_FILE="$(OPEN_PORTS_TO_IGNORE_IN_DOC_TEST_FILE)" OPEN_PORTS_TO_IGNORE_IN_DOC_TEST_FORMAT="$(OPEN_PORTS_TO_IGNORE_IN_DOC_TEST_FORMAT)" \
		$(GINKGO) -v ./test/e2e/...; \
	else \
		echo "Env var 'SUITE' must be set (Options: 'all', 'Validation', 'Nftables')"; \
	fi

# go-install-tool will 'go install' any package $2 and install it to $1.
define go-install-tool
@[ -f $(1) ] || { \
set -e ;\
echo "Downloading $(2)" ;\
GOBIN=$(BIN_DIR) GOFLAGS="" go install $(2) ;\
}
endef
