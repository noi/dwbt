.DEFAULT_GOAL := help

# The directory the GUI opens a .dwbt directory from.
DIR ?= examples/users

WAILS := cd gui && go tool wails
NODE_MODULES := gui/frontend/node_modules/.package-lock.json

.PHONY: help
help: ## Show this help
	@echo 'Usage: make <target> [DIR=<directory containing .dwbt>]'
	@echo
	@awk 'BEGIN {FS = ":.*## "} /^[a-z-]+:.*## / {printf "  %-8s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

.PHONY: setup
setup: $(NODE_MODULES) ## Install the dependencies of the GUI frontend

$(NODE_MODULES): gui/frontend/package-lock.json
	@command -v npm >/dev/null || { echo 'npm is required: install Node.js' >&2; exit 1; }
	cd gui/frontend && npm ci

.PHONY: run
run: setup ## Run the GUI with hot reload, opening DIR
	$(WAILS) dev -appargs $(abspath $(DIR))

.PHONY: build
build: setup ## Build the GUI into gui/build/bin
	$(WAILS) build

.PHONY: test
test: setup ## Run the Go tests of dwbt and the GUI, and type-check the frontend
	go test ./...
	cd gui && go test ./...
	cd gui/frontend && npx tsc --noEmit
